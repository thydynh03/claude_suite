package projectmap

import (
	"strings"
	"testing"

	"claude_suite/backend/models"
)

const goSample = `package sample

import (
	"fmt"

	"claude_suite/backend/textutil"
)

type Widget struct{ Name string }

type Renderer interface{ Render() string }

func (w *Widget) Render() string { return fmt.Sprintf("%s", textutil.Truncate(w.Name, 5, "")) }

func NewWidget(name string) *Widget {
	return &Widget{Name: name}
}

func helper() {}
`

func TestExtractGoFindsFunctionsTypesAndImports(t *testing.T) {
	ex, err := ExtractGo([]byte(goSample), "sample.go")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	byName := map[string]FuncInfo{}
	for _, f := range ex.Functions {
		byName[f.Name] = f
	}

	render, ok := byName["Render"]
	if !ok {
		t.Fatal("method Render not extracted")
	}
	if render.Receiver != "Widget" {
		t.Errorf("Render receiver = %q, want Widget", render.Receiver)
	}
	if !render.Exported {
		t.Error("Render must be exported (uppercase rule)")
	}
	if render.LineStart == 0 || render.LineEnd < render.LineStart {
		t.Errorf("Render line range invalid: %d-%d", render.LineStart, render.LineEnd)
	}

	if h, ok := byName["helper"]; !ok || h.Exported {
		t.Errorf("helper must be extracted and unexported, got %+v", h)
	}

	kinds := map[string]string{}
	for _, c := range ex.Classes {
		kinds[c.Name] = c.Kind
	}
	if kinds["Widget"] != "struct" || kinds["Renderer"] != "interface" {
		t.Errorf("type kinds wrong: %+v", kinds)
	}

	joined := strings.Join(ex.Imports, " ")
	if !strings.Contains(joined, "claude_suite/backend/textutil") {
		t.Errorf("imports missing internal package: %v", ex.Imports)
	}
}

const svelteSample = `<script lang="ts">
import { tasksStore } from '../lib/stores/appState';
import Panel from './Panel.svelte';

export const refresh = () => { console.log('x'); };

function computeRows() {
	return [];
}
</script>

<div>{computeRows()}</div>
`

func TestExtractScriptHandlesSvelte(t *testing.T) {
	ex := ExtractScript([]byte(svelteSample))

	names := map[string]bool{}
	for _, f := range ex.Functions {
		names[f.Name] = true
	}
	if !names["computeRows"] {
		t.Errorf("function computeRows not extracted: %+v", ex.Functions)
	}
	if !names["refresh"] {
		t.Errorf("arrow const refresh not extracted: %+v", ex.Functions)
	}

	joined := strings.Join(ex.Imports, " ")
	if !strings.Contains(joined, "../lib/stores/appState") || !strings.Contains(joined, "./Panel.svelte") {
		t.Errorf("imports not extracted: %v", ex.Imports)
	}
}

// A comment-only edit must classify COSMETIC (no re-extraction, no tokens);
// a signature change must classify STRUCTURAL.
func TestFingerprintClassification(t *testing.T) {
	base := []byte(goSample)
	exBase, _ := ExtractGo(base, "s.go")
	old := fingerprintOf(base, exBase)

	if got := Classify(old, ContentHash(base), StructureSig(exBase)); got != ChangeNone {
		t.Fatalf("identical content classified %v, want NONE", got)
	}

	cosmetic := []byte(strings.Replace(goSample, "func helper() {}", "// một comment mới\nfunc helper() {}", 1))
	exCosmetic, _ := ExtractGo(cosmetic, "s.go")
	if got := Classify(old, ContentHash(cosmetic), StructureSig(exCosmetic)); got != ChangeCosmetic {
		t.Fatalf("comment-only edit classified %v, want COSMETIC", got)
	}

	structural := []byte(strings.Replace(goSample, "func helper() {}", "func helperRenamed() {}", 1))
	exStructural, _ := ExtractGo(structural, "s.go")
	if got := Classify(old, ContentHash(structural), StructureSig(exStructural)); got != ChangeStructural {
		t.Fatalf("signature change classified %v, want STRUCTURAL", got)
	}
}

// Files without an extractor have no structure signature, so any change is
// conservatively STRUCTURAL — the ported Understand-Anything policy.
func TestNoExtractorMeansStructuralOnAnyChange(t *testing.T) {
	old := fingerprintOf([]byte("a: 1\n"), nil)
	if got := Classify(old, ContentHash([]byte("a: 2\n")), ""); got != ChangeStructural {
		t.Fatalf("extractor-less change classified %v, want STRUCTURAL", got)
	}
}

func fingerprintOf(content []byte, ex *Extraction) (fp models.FileFingerprint) {
	fp.ContentHash = ContentHash(content)
	fp.StructureSig = StructureSig(ex)
	return fp
}
