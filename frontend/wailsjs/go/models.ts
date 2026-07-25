export namespace cli {
	
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
	    status: string;
	    tier: string;
	    is_current: boolean;
	    last_used: string;
	    model_quotas: ModelQuota[];
	
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
	        this.status = source["status"];
	        this.tier = source["tier"];
	        this.is_current = source["is_current"];
	        this.last_used = source["last_used"];
	        this.model_quotas = this.convertValues(source["model_quotas"], ModelQuota);
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
	
	    static createFrom(source: any = {}) {
	        return new GitCommitInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.message = source["message"];
	        this.author = source["author"];
	        this.date = source["date"];
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

