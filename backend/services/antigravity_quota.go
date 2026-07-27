package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CloudCodeBase is the Code Assist host Antigravity talks to. A variable so
// tests can point it at a local server, same as GoogleTokenEndpoint.
var CloudCodeBase = "https://cloudcode-pa.googleapis.com"

// ErrNoProject means loadCodeAssist did not hand back a project id.
//
// This is not a detail to shrug off and carry on from. fetchAvailableModels
// called without a project answers 200 with remainingFraction 1.0 for every
// model — including on an account that is being turned away with 429 right now.
// So the failure mode of guessing here is not "no data", it is a full green
// quota bar over a dead account, which is exactly the invented-percentage bug
// this whole area was cleaned up to remove.
var ErrNoProject = errors.New("Google không trả về project id — không thể đọc hạn mức, số trả về sẽ là 100% giả")

// ModelQuotaInfo is one model's remaining quota as Google reported it.
type ModelQuotaInfo struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	// RemainingFraction is 1.0 for untouched and 0 for exhausted. -1 means the
	// response carried no quotaInfo for this model, which is different from 0.
	RemainingFraction float64   `json:"remaining_fraction"`
	ResetTime         time.Time `json:"reset_time"`
	IsExhausted       bool      `json:"is_exhausted"`
	// HasQuota is false when Google listed the model but said nothing about its
	// quota. The UI must render that as a blank, not as a full bar.
	HasQuota bool `json:"has_quota"`
}

// AccountPlan is everything one round of discovery learned about an account.
type AccountPlan struct {
	Tier      string           `json:"tier"`     // FREE | PRO | ULTRA | UNKNOWN
	RawTier   string           `json:"raw_tier"` // what Google literally said
	ProjectID string           `json:"project_id"`
	Models    []ModelQuotaInfo `json:"models"`
	FetchedAt time.Time        `json:"fetched_at"`
}

// cloudCodeHeaders are what the Code Assist endpoints expect from an Antigravity
// client. The User-Agent and X-Goog-Api-Client are not cosmetic: the endpoint is
// v1internal and answers differently to callers it does not recognise.
func cloudCodeHeaders(req *http.Request, accessToken string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity")
	req.Header.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
}

func postCloudCode(ctx context.Context, path, accessToken string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, CloudCodeBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	cloudCodeHeaders(req, accessToken)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("gọi %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 401/403 here is the account's problem, not the network's — the same
		// distinction RefreshGoogleAccessToken draws, and for the same reason:
		// only an answer from Google justifies marking anything about the
		// account, and a timeout must never do so.
		return fmt.Errorf("%w: %s trả về %s", ErrGoogleRefused, path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("đọc phản hồi %s: %w", path, err)
	}
	return nil
}

// LoadCodeAssist asks which project this account bills against and which tier it
// is on. The project id is the part the quota call cannot work without.
func LoadCodeAssist(ctx context.Context, accessToken string) (projectID, rawTier string, err error) {
	payload := map[string]any{
		"metadata": map[string]string{
			"ideType":    "ANTIGRAVITY",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	}
	var body struct {
		CloudaicompanionProject string `json:"cloudaicompanionProject"`
		CurrentTier             struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"currentTier"`
	}
	if err := postCloudCode(ctx, "/v1internal:loadCodeAssist", accessToken, payload, &body); err != nil {
		return "", "", err
	}

	rawTier = body.CurrentTier.ID
	if rawTier == "" {
		rawTier = body.CurrentTier.Name
	}
	return strings.TrimSpace(body.CloudaicompanionProject), strings.TrimSpace(rawTier), nil
}

// FetchAvailableModels reads the per-model quota for one project.
//
// It refuses an empty project id rather than sending the request anyway: see
// ErrNoProject for what Google answers in that case.
func FetchAvailableModels(ctx context.Context, accessToken, projectID string) ([]ModelQuotaInfo, error) {
	if strings.TrimSpace(projectID) == "" {
		return nil, ErrNoProject
	}

	var body struct {
		Models map[string]struct {
			DisplayName string `json:"displayName"`
			QuotaInfo   *struct {
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
				IsExhausted       bool     `json:"isExhausted"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}
	if err := postCloudCode(ctx, "/v1internal:fetchAvailableModels", accessToken, map[string]string{"project": projectID}, &body); err != nil {
		return nil, err
	}

	out := make([]ModelQuotaInfo, 0, len(body.Models))
	for id, m := range body.Models {
		info := ModelQuotaInfo{
			ID:                id,
			DisplayName:       m.DisplayName,
			RemainingFraction: -1,
		}
		if info.DisplayName == "" {
			info.DisplayName = id
		}
		// A model listed with no quotaInfo, or with quotaInfo but no
		// remainingFraction, is unknown — not full. Defaulting a missing number
		// to zero-value 1.0 is how a green bar ends up over an exhausted
		// account, so the pointer is checked rather than the value.
		if m.QuotaInfo != nil && m.QuotaInfo.RemainingFraction != nil {
			info.RemainingFraction = *m.QuotaInfo.RemainingFraction
			info.IsExhausted = m.QuotaInfo.IsExhausted
			info.HasQuota = true
			if t, err := time.Parse(time.RFC3339, m.QuotaInfo.ResetTime); err == nil {
				info.ResetTime = t
			}
		}
		out = append(out, info)
	}

	// Map iteration order is random and this list is rendered as-is.
	sortModelsByName(out)
	return out, nil
}

func sortModelsByName(m []ModelQuotaInfo) {
	for i := 1; i < len(m); i++ {
		for j := i; j > 0 && m[j].DisplayName < m[j-1].DisplayName; j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}

// NormalizeTier maps what Google calls a tier onto the three labels the UI
// filters by. Anything unrecognised stays UNKNOWN rather than being forced into
// the nearest bucket — a wrong label is the thing being avoided here, and the
// raw string is kept alongside so the user can see what was actually said.
func NormalizeTier(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case s == "":
		return "UNKNOWN"
	case strings.Contains(s, "ultra"):
		return "ULTRA"
	case strings.Contains(s, "pro"), strings.Contains(s, "standard"), strings.Contains(s, "paid"), strings.Contains(s, "enterprise"):
		return "PRO"
	case strings.Contains(s, "free"):
		return "FREE"
	default:
		return "UNKNOWN"
	}
}

// FetchAccountPlan runs the whole discovery for one access token: which project,
// which tier, and how much of each model is left.
//
// A tier is still returned when the quota call fails — knowing an account is PRO
// is useful on its own, and losing it because one of two endpoints was unhappy
// would make the button look broken.
func FetchAccountPlan(ctx context.Context, accessToken string) (AccountPlan, error) {
	projectID, rawTier, err := LoadCodeAssist(ctx, accessToken)
	if err != nil {
		return AccountPlan{}, err
	}

	plan := AccountPlan{
		Tier:      NormalizeTier(rawTier),
		RawTier:   rawTier,
		ProjectID: projectID,
		FetchedAt: time.Now(),
	}

	models, err := FetchAvailableModels(ctx, accessToken, projectID)
	if err != nil {
		return plan, err
	}
	plan.Models = models
	return plan, nil
}
