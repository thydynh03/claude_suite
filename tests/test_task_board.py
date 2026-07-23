import os
import sys
import pytest
import tempfile

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '..')))

from modules.task_board import TaskBoard
from modules.models import Task

@pytest.fixture
def temp_board():
    fd, path = tempfile.mkstemp()
    os.close(fd)
    board = TaskBoard(db_path=path)
    yield board
    os.unlink(path)

def test_task_board_counts(temp_board):
    temp_board.add(Task(task_id="1", title="Task 1", status="backlog"))
    temp_board.add(Task(task_id="2", title="Task 2", status="running"))
    temp_board.add(Task(task_id="3", title="Task 3", status="done"))
    temp_board.add(Task(task_id="4", title="Task 4", status="backlog"))
    
    counts = temp_board.counts()
    assert counts.get("backlog", 0) == 2
    assert counts.get("running", 0) == 1
    assert counts.get("done", 0) == 1
    assert counts.get("failed", 0) == 0

def test_task_board_list_all(temp_board):
    t1 = Task(task_id="1", title="Task 1")
    t2 = Task(task_id="2", title="Task 2")
    temp_board.add(t1)
    temp_board.add(t2)
    
    all_tasks = temp_board.list_all()
    assert len(all_tasks) == 2

def test_dependencies(temp_board):
    import json
    
    # Task 1 (Done)
    t1 = Task(task_id="1", title="Task 1", status="done")
    temp_board.add(t1)
    
    # Task 2 (Backlog, depends on Task 1) -> Should be dispatchable
    t2 = Task(task_id="2", title="Task 2", status="backlog", depends_on=json.dumps(["1"]))
    temp_board.add(t2)
    
    # Task 3 (Backlog, depends on Task 4) -> Should NOT be dispatchable
    t3 = Task(task_id="3", title="Task 3", status="backlog", depends_on=json.dumps(["4"]))
    temp_board.add(t3)
    
    # Task 4 (Backlog, no dependencies) -> Should be dispatchable
    t4 = Task(task_id="4", title="Task 4", status="backlog")
    temp_board.add(t4)
    
    dispatchable = temp_board.next_dispatchable()
    dispatchable_ids = [t.task_id for t in dispatchable]
    
    assert "2" in dispatchable_ids
    assert "4" in dispatchable_ids
    assert "3" not in dispatchable_ids
