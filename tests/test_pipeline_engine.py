import os
import sys
import pytest

sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), '..')))

from modules.pipeline_engine import AgentPipeline
from modules.agent_registry import AgentRegistry
from modules.models import Agent

def test_pipeline_engine_auto_provision():
    import tempfile
    fd, path = tempfile.mkstemp()
    os.close(fd)
    
    registry = AgentRegistry(db_path=path)
    # Start with empty registry
    registry.delete_all()
    
    pipeline = AgentPipeline(registry, None, None)
    pipeline.on_log = lambda msg, lvl="INFO": None
    
    # Empty agents list
    agents = []
    
    # Should auto-provision
    agent = pipeline._find_agent_for_role("Senior Tester", agents)
    
    assert agent is not None
    assert agent.name == "[Auto] Senior Tester"
    assert len(agents) == 1
    
    # Should use the newly added agent next time
    agent2 = pipeline._find_agent_for_role("Senior Tester", agents)
    assert agent2.agent_id == agent.agent_id
    assert len(agents) == 1
    
    os.unlink(path)
