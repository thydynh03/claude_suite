export namespace cli {
	
	export class RunResult {
	    success: boolean;
	    output: string;
	    error: string;
	    session_id: string;
	    tokens_used: number;
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
	        this.duration_sec = source["duration_sec"];
	    }
	}

}

export namespace main {
	
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
	
	export class ScheduledJob {
	    id: string;
	    prompt: string;
	    // Go type: time
	    target_time: any;
	    repeat: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ScheduledJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.prompt = source["prompt"];
	        this.target_time = this.convertValues(source["target_time"], null);
	        this.repeat = source["repeat"];
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

