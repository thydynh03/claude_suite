export namespace claims {
	
	export class ChatMessage {
	    seq: number;
	    author: string;
	    text: string;
	    // Go type: time
	    at: any;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.author = source["author"];
	        this.text = source["text"];
	        this.at = this.convertValues(source["at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Claim {
	    id: string;
	    session_id: string;
	    author: string;
	    provider: string;
	    subject: string;
	    assertion: string;
	    falsifier: string;
	    kind: string;
	    verdict: string;
	    evidence: string;
	    exit_code: number;
	    // Go type: time
	    submitted_at: any;
	    // Go type: time
	    settled_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new Claim(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.session_id = source["session_id"];
	        this.author = source["author"];
	        this.provider = source["provider"];
	        this.subject = source["subject"];
	        this.assertion = source["assertion"];
	        this.falsifier = source["falsifier"];
	        this.kind = source["kind"];
	        this.verdict = source["verdict"];
	        this.evidence = source["evidence"];
	        this.exit_code = source["exit_code"];
	        this.submitted_at = this.convertValues(source["submitted_at"], null);
	        this.settled_at = this.convertValues(source["settled_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Participant {
	    author: string;
	    provider: string;
	    // Go type: time
	    joined_at: any;
	    // Go type: time
	    last_seen: any;
	
	    static createFrom(source: any = {}) {
	        return new Participant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.author = source["author"];
	        this.provider = source["provider"];
	        this.joined_at = this.convertValues(source["joined_at"], null);
	        this.last_seen = this.convertValues(source["last_seen"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Remark {
	    round: number;
	    author: string;
	    claim_id: string;
	    text: string;
	    // Go type: time
	    at: any;
	
	    static createFrom(source: any = {}) {
	        return new Remark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.round = source["round"];
	        this.author = source["author"];
	        this.claim_id = source["claim_id"];
	        this.text = source["text"];
	        this.at = this.convertValues(source["at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Outcome {
	    session_id: string;
	    subject: string;
	    // Go type: time
	    closed_at: any;
	    blocking: Claim[];
	    refuted: Claim[];
	    escalated: Claim[];
	    dissent: Remark[];
	    warnings: string[];
	    participants: Participant[];
	
	    static createFrom(source: any = {}) {
	        return new Outcome(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session_id = source["session_id"];
	        this.subject = source["subject"];
	        this.closed_at = this.convertValues(source["closed_at"], null);
	        this.blocking = this.convertValues(source["blocking"], Claim);
	        this.refuted = this.convertValues(source["refuted"], Claim);
	        this.escalated = this.convertValues(source["escalated"], Claim);
	        this.dissent = this.convertValues(source["dissent"], Remark);
	        this.warnings = source["warnings"];
	        this.participants = this.convertValues(source["participants"], Participant);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	

}

export namespace cli {
	
	export class UsageStats {
	    requests: number;
	    rate_limit_hits: number;
	    // Go type: time
	    last_rate_limit_at: any;
	    // Go type: time
	    last_request_at: any;
	
	    static createFrom(source: any = {}) {
	        return new UsageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.requests = source["requests"];
	        this.rate_limit_hits = source["rate_limit_hits"];
	        this.last_rate_limit_at = this.convertValues(source["last_rate_limit_at"], null);
	        this.last_request_at = this.convertValues(source["last_request_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModelQuota {
	    name: string;
	    category: string;
	    reset_time: string;
	    usage_pct: number;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelQuota(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.category = source["category"];
	        this.reset_time = source["reset_time"];
	        this.usage_pct = source["usage_pct"];
	        this.status = source["status"];
	    }
	}
	export class AntiAccountKey {
	    id: string;
	    name: string;
	    email: string;
	    type: string;
	    api_key: string;
	    oauth_token: string;
	    refresh_token: string;
	    // Go type: time
	    token_expires_at: any;
	    status: string;
	    tier: string;
	    is_current: boolean;
	    last_used: string;
	    model_quotas: ModelQuota[];
	    usage: UsageStats;
	    tier_source: string;
	
	    static createFrom(source: any = {}) {
	        return new AntiAccountKey(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.email = source["email"];
	        this.type = source["type"];
	        this.api_key = source["api_key"];
	        this.oauth_token = source["oauth_token"];
	        this.refresh_token = source["refresh_token"];
	        this.token_expires_at = this.convertValues(source["token_expires_at"], null);
	        this.status = source["status"];
	        this.tier = source["tier"];
	        this.is_current = source["is_current"];
	        this.last_used = source["last_used"];
	        this.model_quotas = this.convertValues(source["model_quotas"], ModelQuota);
	        this.usage = this.convertValues(source["usage"], UsageStats);
	        this.tier_source = source["tier_source"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RunResult {
	    success: boolean;
	    output: string;
	    error: string;
	    session_id: string;
	    tokens_used: number;
	    cost_usd: number;
	    duration_sec: number;
	    account_id: string;
	    usage_estimated: boolean;
	    model_used: string;
	
	    static createFrom(source: any = {}) {
	        return new RunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.output = source["output"];
	        this.error = source["error"];
	        this.session_id = source["session_id"];
	        this.tokens_used = source["tokens_used"];
	        this.cost_usd = source["cost_usd"];
	        this.duration_sec = source["duration_sec"];
	        this.account_id = source["account_id"];
	        this.usage_estimated = source["usage_estimated"];
	        this.model_used = source["model_used"];
	    }
	}

}

export namespace database {
	
	export class Blocker {
	    task_id: string;
	    title: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new Blocker(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.title = source["title"];
	        this.status = source["status"];
	    }
	}
	export class BlockedTask {
	    task_id: string;
	    title: string;
	    blockers: Blocker[];
	    dead: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BlockedTask(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.title = source["title"];
	        this.blockers = this.convertValues(source["blockers"], Blocker);
	        this.dead = source["dead"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class DispatchReadiness {
	    ready: number;
	    blocked: BlockedTask[];
	    running: number;
	
	    static createFrom(source: any = {}) {
	        return new DispatchReadiness(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ready = source["ready"];
	        this.blocked = this.convertValues(source["blocked"], BlockedTask);
	        this.running = source["running"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class BudgetStatus {
	    day: string;
	    spent_usd: number;
	    limit_usd: number;
	
	    static createFrom(source: any = {}) {
	        return new BudgetStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.spent_usd = source["spent_usd"];
	        this.limit_usd = source["limit_usd"];
	    }
	}
	export class SystemMetrics {
	    alloc_memory_mb: number;
	    sys_memory_mb: number;
	    num_goroutine: number;
	    num_cpu: number;
	    active_keys_count: number;
	
	    static createFrom(source: any = {}) {
	        return new SystemMetrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alloc_memory_mb = source["alloc_memory_mb"];
	        this.sys_memory_mb = source["sys_memory_mb"];
	        this.num_goroutine = source["num_goroutine"];
	        this.num_cpu = source["num_cpu"];
	        this.active_keys_count = source["active_keys_count"];
	    }
	}
	export class UpdateResponse {
	    success: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	}
	export class WarmupResult {
	    total: number;
	    refreshed: number;
	    failed: number;
	    skipped: number;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new WarmupResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.refreshed = source["refreshed"];
	        this.failed = source["failed"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	    }
	}

}

export namespace modelcatalog {
	
	export class Model {
	    id: string;
	    label: string;
	    provider: string;
	    source: string;
	    token_limit: number;
	
	    static createFrom(source: any = {}) {
	        return new Model(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.provider = source["provider"];
	        this.source = source["source"];
	        this.token_limit = source["token_limit"];
	    }
	}

}

export namespace models {
	
	export class Agent {
	    agent_id: string;
	    name: string;
	    role: string;
	    provider: string;
	    model: string;
	    system: string;
	    icon: string;
	    session_id: string;
	    status: string;
	    tasks_done: number;
	    last_task: string;
	    last_error: string;
	    notes: string;
	    tokens_used: number;
	    token_limit: number;
	    token_remaining: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Agent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent_id = source["agent_id"];
	        this.name = source["name"];
	        this.role = source["role"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.system = source["system"];
	        this.icon = source["icon"];
	        this.session_id = source["session_id"];
	        this.status = source["status"];
	        this.tasks_done = source["tasks_done"];
	        this.last_task = source["last_task"];
	        this.last_error = source["last_error"];
	        this.notes = source["notes"];
	        this.tokens_used = source["tokens_used"];
	        this.token_limit = source["token_limit"];
	        this.token_remaining = source["token_remaining"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IntegrationsConfig {
	    outbound_webhook_url: string;
	    mcp_connection_string: string;
	
	    static createFrom(source: any = {}) {
	        return new IntegrationsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outbound_webhook_url = source["outbound_webhook_url"];
	        this.mcp_connection_string = source["mcp_connection_string"];
	    }
	}
	export class MemoryConfig {
	    context_pack_max_chars: number;
	    auto_summarize: boolean;
	    session_resume: boolean;
	    lesson_promotion: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MemoryConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.context_pack_max_chars = source["context_pack_max_chars"];
	        this.auto_summarize = source["auto_summarize"];
	        this.session_resume = source["session_resume"];
	        this.lesson_promotion = source["lesson_promotion"];
	    }
	}
	export class MemoryLesson {
	    lesson_id: string;
	    workspace_id: string;
	    kind: string;
	    trigger: string;
	    action: string;
	    evidence: string;
	    confidence: number;
	    status: string;
	    source_task_id: string;
	    use_count: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    // Go type: time
	    last_used_at: any;
	
	    static createFrom(source: any = {}) {
	        return new MemoryLesson(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lesson_id = source["lesson_id"];
	        this.workspace_id = source["workspace_id"];
	        this.kind = source["kind"];
	        this.trigger = source["trigger"];
	        this.action = source["action"];
	        this.evidence = source["evidence"];
	        this.confidence = source["confidence"];
	        this.status = source["status"];
	        this.source_task_id = source["source_task_id"];
	        this.use_count = source["use_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.last_used_at = this.convertValues(source["last_used_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModuleGraphEdge {
	    source: string;
	    target: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new ModuleGraphEdge(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.target = source["target"];
	        this.count = source["count"];
	    }
	}
	export class ModuleGraphNode {
	    id: string;
	    name: string;
	    files: number;
	
	    static createFrom(source: any = {}) {
	        return new ModuleGraphNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.files = source["files"];
	    }
	}
	export class ModuleGraph {
	    nodes: ModuleGraphNode[];
	    edges: ModuleGraphEdge[];
	
	    static createFrom(source: any = {}) {
	        return new ModuleGraph(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.nodes = this.convertValues(source["nodes"], ModuleGraphNode);
	        this.edges = this.convertValues(source["edges"], ModuleGraphEdge);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class PipelineStep {
	    step_id: number;
	    stage_name: string;
	    role: string;
	    agent_id: string;
	    prompt: string;
	    output: string;
	    status: string;
	    duration_sec: number;
	
	    static createFrom(source: any = {}) {
	        return new PipelineStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step_id = source["step_id"];
	        this.stage_name = source["stage_name"];
	        this.role = source["role"];
	        this.agent_id = source["agent_id"];
	        this.prompt = source["prompt"];
	        this.output = source["output"];
	        this.status = source["status"];
	        this.duration_sec = source["duration_sec"];
	    }
	}
	export class StalenessReport {
	    status: string;
	    graph_commit: string;
	    head_commit: string;
	    commits_behind: number;
	    dirty: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StalenessReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.graph_commit = source["graph_commit"];
	        this.head_commit = source["head_commit"];
	        this.commits_behind = source["commits_behind"];
	        this.dirty = source["dirty"];
	    }
	}
	export class ProjectMapStats {
	    workspace_id: string;
	    nodes: number;
	    edges: number;
	    files: number;
	    // Go type: time
	    analyzed_at: any;
	    git_commit_hash: string;
	    staleness: StalenessReport;
	
	    static createFrom(source: any = {}) {
	        return new ProjectMapStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workspace_id = source["workspace_id"];
	        this.nodes = source["nodes"];
	        this.edges = source["edges"];
	        this.files = source["files"];
	        this.analyzed_at = this.convertValues(source["analyzed_at"], null);
	        this.git_commit_hash = source["git_commit_hash"];
	        this.staleness = this.convertValues(source["staleness"], StalenessReport);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Regression {
	    regression_id: string;
	    workspace_id: string;
	    title: string;
	    symptom: string;
	    root_cause: string;
	    fix_summary: string;
	    files: string[];
	    guard_check_id: string;
	    guard_status: string;
	    failed_task_id: string;
	    fixed_task_id: string;
	    status: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Regression(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.regression_id = source["regression_id"];
	        this.workspace_id = source["workspace_id"];
	        this.title = source["title"];
	        this.symptom = source["symptom"];
	        this.root_cause = source["root_cause"];
	        this.fix_summary = source["fix_summary"];
	        this.files = source["files"];
	        this.guard_check_id = source["guard_check_id"];
	        this.guard_status = source["guard_status"];
	        this.failed_task_id = source["failed_task_id"];
	        this.fixed_task_id = source["fixed_task_id"];
	        this.status = source["status"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Task {
	    task_id: string;
	    title: string;
	    description: string;
	    prompt: string;
	    priority: string;
	    status: string;
	    assigned_to: string;
	    depends_on: string[];
	    retry_count: number;
	    max_retries: number;
	    result: string;
	    session_id: string;
	    parent_id: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    started_at: any;
	    // Go type: time
	    finished_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.prompt = source["prompt"];
	        this.priority = source["priority"];
	        this.status = source["status"];
	        this.assigned_to = source["assigned_to"];
	        this.depends_on = source["depends_on"];
	        this.retry_count = source["retry_count"];
	        this.max_retries = source["max_retries"];
	        this.result = source["result"];
	        this.session_id = source["session_id"];
	        this.parent_id = source["parent_id"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.finished_at = this.convertValues(source["finished_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorkspaceConfig {
	    last_workspace_folder: string;
	    recent_workspaces: string[];
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.last_workspace_folder = source["last_workspace_folder"];
	        this.recent_workspaces = source["recent_workspaces"];
	    }
	}

}

export namespace projectmap {
	
	export class BuildReport {
	    workspace_id: string;
	    files: number;
	    nodes: number;
	    edges: number;
	    duration_sec: number;
	
	    static createFrom(source: any = {}) {
	        return new BuildReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.workspace_id = source["workspace_id"];
	        this.files = source["files"];
	        this.nodes = source["nodes"];
	        this.edges = source["edges"];
	        this.duration_sec = source["duration_sec"];
	    }
	}

}

export namespace services {
	
	export class BrowserActionResult {
	    url: string;
	    title: string;
	    html_snippet: string;
	    text_content: string;
	    screenshot_base64: string;
	    console_errors: string[];
	    assertions: string[];
	    passed: boolean;
	    logs: string[];
	    success: boolean;
	    error: string;
	    ai_response: string;
	    status: string;
	    current_step: number;
	    max_steps: number;
	    user_intervention: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserActionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.title = source["title"];
	        this.html_snippet = source["html_snippet"];
	        this.text_content = source["text_content"];
	        this.screenshot_base64 = source["screenshot_base64"];
	        this.console_errors = source["console_errors"];
	        this.assertions = source["assertions"];
	        this.passed = source["passed"];
	        this.logs = source["logs"];
	        this.success = source["success"];
	        this.error = source["error"];
	        this.ai_response = source["ai_response"];
	        this.status = source["status"];
	        this.current_step = source["current_step"];
	        this.max_steps = source["max_steps"];
	        this.user_intervention = source["user_intervention"];
	    }
	}
	export class GitBranchInfo {
	    current: string;
	    branches: string[];
	
	    static createFrom(source: any = {}) {
	        return new GitBranchInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.branches = source["branches"];
	    }
	}
	export class GitCommitInfo {
	    hash: string;
	    message: string;
	    author: string;
	    date: string;
	    parents: string[];
	    refs: string[];
	
	    static createFrom(source: any = {}) {
	        return new GitCommitInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.message = source["message"];
	        this.author = source["author"];
	        this.date = source["date"];
	        this.parents = source["parents"];
	        this.refs = source["refs"];
	    }
	}
	export class GitWatchConfig {
	    enabled: boolean;
	    threshold_hours: number;
	    auto_commit: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GitWatchConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.threshold_hours = source["threshold_hours"];
	        this.auto_commit = source["auto_commit"];
	    }
	}
	export class MCPServer {
	    id: string;
	    name: string;
	    description: string;
	    command: string;
	    args: string[];
	    env: Record<string, string>;
	    url: string;
	    enabled: boolean;
	    builtin: boolean;
	    // Go type: time
	    last_checked_at: any;
	    last_status: string;
	    last_error: string;
	
	    static createFrom(source: any = {}) {
	        return new MCPServer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.url = source["url"];
	        this.enabled = source["enabled"];
	        this.builtin = source["builtin"];
	        this.last_checked_at = this.convertValues(source["last_checked_at"], null);
	        this.last_status = source["last_status"];
	        this.last_error = source["last_error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ScheduledJob {
	    id: string;
	    prompt: string;
	    // Go type: time
	    target_time: any;
	    repeat: boolean;
	    provider: string;
	    model: string;
	    kind: string;
	    enabled: boolean;
	    wait_for_quota: boolean;
	    // Go type: time
	    last_run_at: any;
	    last_status: string;
	    last_error: string;
	    run_count: number;
	
	    static createFrom(source: any = {}) {
	        return new ScheduledJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	        this.target_time = this.convertValues(source["target_time"], null);
	        this.repeat = source["repeat"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.kind = source["kind"];
	        this.enabled = source["enabled"];
	        this.wait_for_quota = source["wait_for_quota"];
	        this.last_run_at = this.convertValues(source["last_run_at"], null);
	        this.last_status = source["last_status"];
	        this.last_error = source["last_error"];
	        this.run_count = source["run_count"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UpdateInfo {
	    has_update: boolean;
	    version: string;
	    download_url: string;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.has_update = source["has_update"];
	        this.version = source["version"];
	        this.download_url = source["download_url"];
	        this.body = source["body"];
	    }
	}

}

