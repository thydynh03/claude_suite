export interface Agent {
  agent_id: string;
  name: string;
  role: string;
  provider: string;
  model: string;
  system: string;
  icon: string;
  session_id: string;
  status: string; // "idle" | "running" | "error"
  tasks_done: number;
  last_task: string;
  last_error: string;
  notes: string;
  tokens_used: number;
  token_limit: number;
  token_remaining: number;
}

export interface Task {
  task_id: string;
  title: string;
  description: string;
  prompt: string;
  priority: string; // "high" | "normal" | "low"
  status: string;   // "backlog" | "queued" | "running" | "done" | "failed"
  assigned_to: string;
  depends_on: string[];
  retry_count: number;
  max_retries: number;
  result: string;
  session_id: string;
  parent_id: string;
  created_at?: string;
  started_at?: string;
  finished_at?: string;
}

export interface PipelineStep {
  step_id: number;
  stage_name: string;
  role: string;
  agent_id: string;
  prompt: string;
  output: string;
  status: string; // "pending" | "running" | "completed" | "error"
  duration_sec: number;
}

export interface LogEntry {
  message: string;
  level: string; // "INFO" | "SUCCESS" | "WARN" | "ERROR" | "THINKING" | "SEND"
  time: string;
  // Unique per entry within the session. Streamed lines repeat — same second,
  // same text — so nothing derived from the content is usable as an each-block
  // key: Svelte 5 throws each_key_duplicate mid-render, which left the task
  // drawer's full-screen overlay stuck over a dead UI.
  seq?: number;
}

export interface WorkspaceConfig {
  last_workspace_folder: string;
  recent_workspaces: string[];
}
