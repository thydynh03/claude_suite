import tkinter as tk
import customtkinter as ctk
import time
import math
import random

class VirtualOfficeTabFrame(ctk.CTkFrame):
    def __init__(self, parent, app, **kwargs):
        super().__init__(parent, **kwargs)
        self.app = app
        
        self.canvas = tk.Canvas(self, bg="#1e293b", highlightthickness=0)
        self.canvas.pack(fill="both", expand=True, padx=10, pady=10)
        
        self.btn_frame = ctk.CTkFrame(self, fg_color="transparent")
        self.btn_frame.pack(fill="x", padx=10, pady=(0, 10))
        
        self.btn_demo = ctk.CTkButton(self.btn_frame, text="▶ Chạy thử Demo (Run Animation)", command=self._run_demo, fg_color="#8b5cf6", hover_color="#7c3aed")
        self.btn_demo.pack(side="right")
        
        # Desks and Agents
        self.desks = {}
        self.agents = {}
        self.bubbles = []
        
        self.width = 1000
        self.height = 600
        
        # Setup Layout
        self._setup_office()
        
        # Start Animation Loop
        self.running = True
        self._last_time = time.time()
        self._animate()
        
    def _setup_office(self):
        # We define a few fixed positions for the desks
        desk_positions = {
            "CEO & Executive Director": (200, 150),
            "Technical Project Manager (PM)": (450, 150),
            "Lead Business Analyst (BA)": (700, 150),
            "Chief Architect & Tech Lead": (200, 400),
            "Senior Fullstack Developer": (450, 400),
            "Senior Code Reviewer & Security Auditor": (700, 400),
            "Lead QA & Test Automation Specialist": (850, 400)
        }
        
        # Default fallback positions
        idx = 0
        
        current_agents = self.app.registry.list_all()
        
        for agent_obj in current_agents:
            name = agent_obj.name
            if name in desk_positions:
                x, y = desk_positions[name]
            else:
                x, y = (100 + (idx % 4) * 200, 250 + (idx // 4) * 200)
                idx += 1
                
            self.desks[name] = {"x": x, "y": y, "name": name, "desk_id": None, "laptop_id": None}
            self._draw_desk(x, y, name)
            
            # Spawn Agent
            self.agents[name] = {
                "name": name,
                "home_x": x,
                "home_y": y,
                "curr_x": x,
                "curr_y": y,
                "target_x": x,
                "target_y": y,
                "state": "idle",
                "emoji": "🤖",
                "sprite_id": self.canvas.create_text(x, y, text="🤖", font=("Segoe UI Emoji", 24)),
                "label_id": self.canvas.create_text(x, y + 25, text=name.split()[0], fill="white", font=("Arial", 9))
            }
            
            # Emoji map
            if "CEO" in name: self.agents[name]["emoji"] = "🤵"
            elif "BA" in name: self.agents[name]["emoji"] = "👩‍💼"
            elif "PM" in name: self.agents[name]["emoji"] = "👨‍💼"
            elif "Architect" in name: self.agents[name]["emoji"] = "🧙‍♂️"
            elif "Developer" in name: self.agents[name]["emoji"] = "👨‍💻"
            elif "Reviewer" in name: self.agents[name]["emoji"] = "🕵️‍♂️"
            elif "QA" in name: self.agents[name]["emoji"] = "👩‍🔬"
            
            self.canvas.itemconfig(self.agents[name]["sprite_id"], text=self.agents[name]["emoji"])
            
    def _draw_desk(self, x, y, name):
        # Draw desk
        d_id = self.canvas.create_rectangle(x - 40, y - 20, x + 40, y + 20, fill="#334155", outline="#475569", width=2)
        # Draw laptop
        l_id = self.canvas.create_rectangle(x - 15, y - 10, x + 15, y + 5, fill="#94a3b8")
        self.desks[name]["desk_id"] = d_id
        self.desks[name]["laptop_id"] = l_id
        
    def refresh_office(self):
        self.canvas.delete("all")
        self.desks.clear()
        self.agents.clear()
        self.bubbles.clear()
        self._setup_office()
        
    def _animate(self):
        if not self.running:
            return
            
        now = time.time()
        
        # Poll for agent registry changes
        if int(now) % 2 == 0:  # Check roughly every 2 seconds (using an attribute for throttling would be better, but this is simple)
            current_agents = self.app.registry.list_all()
            if len(current_agents) != len(self.agents) or any(a.name not in self.agents for a in current_agents):
                self.refresh_office()
                self.after(33, self._animate)
                return
                
        dt = now - self._last_time
        dt = now - self._last_time
        self._last_time = now
        
        speed = 200 # pixels per second
        
        # Update Agents
        for name, agent in self.agents.items():
            cx, cy = agent["curr_x"], agent["curr_y"]
            tx, ty = agent["target_x"], agent["target_y"]
            
            if abs(tx - cx) > 1 or abs(ty - cy) > 1:
                # Move towards target
                dx = tx - cx
                dy = ty - cy
                dist = math.hypot(dx, dy)
                
                move_dist = speed * dt
                if move_dist > dist:
                    cx, cy = tx, ty
                else:
                    cx += (dx / dist) * move_dist
                    cy += (dy / dist) * move_dist
                    
                agent["curr_x"] = cx
                agent["curr_y"] = cy
                
                # Update canvas
                self.canvas.coords(agent["sprite_id"], cx, cy)
                self.canvas.coords(agent["label_id"], cx, cy + 25)
            else:
                # Arrived
                if agent["state"] == "walking_to_desk":
                    agent["state"] = "chatting"
                elif agent["state"] == "walking_home":
                    agent["state"] = "idle"
                    
        # Update Bubbles
        active_bubbles = []
        for b in self.bubbles:
            b["life"] -= dt
            if b["life"] <= 0:
                self.canvas.delete(b["bg_id"])
                self.canvas.delete(b["text_id"])
            else:
                # Float up slightly
                self.canvas.move(b["bg_id"], 0, -10 * dt)
                self.canvas.move(b["text_id"], 0, -10 * dt)
                active_bubbles.append(b)
        self.bubbles = active_bubbles
        
        self.after(33, self._animate)
        
    def trigger_communication(self, from_agent, to_agent, message, duration=3.0):
        if from_agent not in self.agents or to_agent not in self.agents:
            return
            
        agent = self.agents[from_agent]
        target = self.agents[to_agent]
        
        # Agent stands up and walks to target's desk
        agent["target_x"] = target["home_x"] - 60 # Stand next to their desk
        agent["target_y"] = target["home_y"]
        agent["state"] = "walking_to_desk"
        
        # Show chat bubble immediately
        self.show_bubble(from_agent, message, duration)
        
        # Schedule return trip
        self.after(int((duration + 1) * 1000), lambda: self._return_home(from_agent))
        
    def show_bubble(self, agent_name, text, duration=3.0):
        if agent_name not in self.agents:
            return
        agent = self.agents[agent_name]
        
        # Wrap text
        words = text.split()
        wrapped = ""
        line = ""
        for w in words:
            if len(line) + len(w) > 30:
                wrapped += line + "\n"
                line = w + " "
            else:
                line += w + " "
        wrapped += line
        
        x, y = agent["curr_x"], agent["curr_y"] - 40
        
        text_id = self.canvas.create_text(x, y, text=wrapped, fill="white", font=("Arial", 10), justify="center")
        bbox = self.canvas.bbox(text_id)
        pad = 5
        bg_id = self.canvas.create_rectangle(bbox[0]-pad, bbox[1]-pad, bbox[2]+pad, bbox[3]+pad, fill="#475569", outline="#38bdf8", width=1,  tags="bubble")
        
        self.canvas.tag_raise(text_id, bg_id)
        
        self.bubbles.append({
            "bg_id": bg_id,
            "text_id": text_id,
            "life": duration
        })
        
    def _return_home(self, agent_name):
        if agent_name in self.agents:
            agent = self.agents[agent_name]
            agent["target_x"] = agent["home_x"]
            agent["target_y"] = agent["home_y"]
            agent["state"] = "walking_home"

    def assign_task(self, agent_name, task_title):
        self.show_bubble(agent_name, f"Working on:\n{task_title}", 5.0)

    def complete_task(self, agent_name):
        self.show_bubble(agent_name, "✅ Task Completed!", 3.0)
        
    def _run_demo(self):
        # Fake animation sequence
        if not self.agents:
            return
            
        agent_names = list(self.agents.keys())
        a1 = agent_names[0]
        a2 = agent_names[1 % len(agent_names)]
        a3 = agent_names[2 % len(agent_names)]
        
        self.assign_task(a1, "Thiết kế System Architecture")
        
        self.after(2000, lambda: self.trigger_communication(a1, a2, "Bản vẽ xong rồi, code đi em!"))
        
        self.after(5000, lambda: self.assign_task(a2, "Code REST API..."))
        
        self.after(8000, lambda: self.trigger_communication(a2, a3, "Review giúp đoạn auth nha"))
        
        self.after(11000, lambda: self.assign_task(a3, "Viết Cypress E2E test"))
        
        self.after(14000, lambda: self.trigger_communication(a3, a1, "Test pass hết rồi sếp!"))
        
        self.after(17000, lambda: self.complete_task(a1))

    def destroy(self):
        self.running = False
        super().destroy()
