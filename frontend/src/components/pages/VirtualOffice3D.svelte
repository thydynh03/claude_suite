<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as THREE from 'three';
  import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
  import { logs, agentsStore } from '../../lib/stores/appState';

  let container: HTMLDivElement;
  let scene: THREE.Scene;
  let camera: THREE.OrthographicCamera;
  let renderer: THREE.WebGLRenderer;
  let animId: number;
  let controls: OrbitControls;

  let isSimulating = false;
  let isAllSeatsPopulated = false;
  let globalBubble = 'REALISTIC OFFICE: Photorealistic PBR Office Architecture Active';

  let agents3D: { mesh: THREE.Group; name: string; targetX: number; targetZ: number; targetRotY?: number; bubbleText: string; parts?: any; isTyping?: boolean }[] = [];
  let uiAgents: { id: number; name: string; text: string; x: number; y: number; visible: boolean }[] = [];

  let deskObjects: THREE.Object3D[] = [];
  let currentAgentList: any[] = [];

  const unsubscribeAgents = agentsStore.subscribe((list) => {
    currentAgentList = list || [];
    if (scene && !isAllSeatsPopulated) {
      rebuildOfficeLayout();
    }
  });

  function handleVisibilityChange() {
    if (document.hidden) {
      if (animId) cancelAnimationFrame(animId);
    } else {
      animate();
    }
  }

  onMount(() => {
    init3D();
    window.addEventListener('resize', handleResize);
    document.addEventListener('visibilitychange', handleVisibilityChange);
  });

  onDestroy(() => {
    unsubscribeAgents();
    if (animId) cancelAnimationFrame(animId);
    window.removeEventListener('resize', handleResize);
    document.removeEventListener('visibilitychange', handleVisibilityChange);
  });

  // Pre-built 18 Staff Workstation Pod Positions (3 rows of 6 cubicles)
  const staffPodPositions = [
    { x: -21, z: -11 }, { x: -15, z: -11 }, { x: -9, z: -11 }, { x: -3, z: -11 }, { x: 3, z: -11 }, { x: -21, z: -6 },
    { x: -15, z: -6 },  { x: -9, z: -6 },  { x: -3, z: -6 },  { x: 3, z: -6 },  { x: -21, z: -1 }, { x: -15, z: -1 },
    { x: -9, z: -1 },   { x: -3, z: -1 },   { x: 3, z: -1 },   { x: -21, z: 4 },  { x: -15, z: 4 },  { x: -9, z: 4 }
  ];

  // Button 1: Populate All 35 Seats with Realistic Demo Avatars
  function handlePopulateAllSeats() {
    isAllSeatsPopulated = true;
    agents3D.forEach(a => scene.remove(a.mesh));
    agents3D = [];

    const corporateRoles = [
      "Sếp Tổng (CEO)", "VP Engineering", "Tech Lead Architect", "Lead Product Manager",
      "Lead Frontend Engineer", "Senior Backend Go", "Senior Backend Python", "DevOps Cloud Architect",
      "Fullstack Specialist", "QA Automation Lead", "UI/UX Designer", "Database Architect",
      "Security Analyst", "System Engineer", "Data Engineer", "Mobile Flutter Developer",
      "AI Prompt Specialist", "Site Reliability Engineer", "Scrum Master", "Business Analyst",
      "Junior Backend", "Junior Frontend", "Manual Tester", "API Integration Lead",
      "Lễ Tân (Receptionist)"
    ];

    const colors = [0x0284c7, 0x7c3aed, 0xd97706, 0x059669, 0xdc2626, 0xdb2777, 0x4f46e5, 0x0d9488, 0x0891b2];

    // 1. CEO Executive Suite (4 Seats)
    createCharacterAvatar(corporateRoles[0], 0xd97706, 13, -9.4, Math.PI); // CEO Chair facing desk North
    createCharacterAvatar(corporateRoles[1], 0x7c3aed, 10, -7, Math.PI / 2); // Guest Armchair 1
    createCharacterAvatar(corporateRoles[2], 0x0284c7, 16, -7, -Math.PI / 2); // Guest Armchair 2
    createCharacterAvatar(corporateRoles[18], 0x4f46e5, 8, -7, Math.PI / 2);  // Executive Sofa

    // 2. Conference Room (12 Chairs)
    for (let i = 0; i < 12; i++) {
      const angle = (i / 12) * Math.PI * 2;
      const cx = 13 + Math.cos(angle) * 5.0;
      const cz = 8 + Math.sin(angle) * 2.8;
      const rotY = -angle + Math.PI / 2;
      const role = corporateRoles[(i + 3) % corporateRoles.length];
      createCharacterAvatar(role, colors[i % colors.length], cx, cz, rotY);
    }

    // 3. Reception Desk (1 Seat)
    createCharacterAvatar("Lễ Tân (Receptionist)", 0xdb2777, 0, 13.2, 0);

    // 4. Staff Workstation Cubicles (18 Seats)
    staffPodPositions.forEach((pod, idx) => {
      const role = corporateRoles[(idx + 6) % corporateRoles.length];
      createCharacterAvatar(role, colors[idx % colors.length], pod.x, pod.z + 0.9, Math.PI);
    });

    globalBubble = `REALISTIC OFFICE: All 35 Seats Populated Facing Dual Monitors!`;
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'SYSTEM', message: `Populated all 35 seats facing keyboards & dual monitors correctly` }];
  }

  // Button 2: Run Full Office Corporate Scenario Simulation
  async function handleRunFullOfficeScenario() {
    if (isSimulating) return;
    if (agents3D.length < 5) {
      handlePopulateAllSeats();
    }

    isSimulating = true;
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'INFO', message: `Initiating Full-Office Realistic Corporate Simulation (4 Phases)` }];

    // Phase 1: Morning Coffee & Watercooler Standup
    globalBubble = 'PHASE 1: Morning Coffee & Team Watercooler Catchup at Bar';
    const coffeeBarAgents = agents3D.slice(0, 8);
    const originalPositions = coffeeBarAgents.map(a => ({ x: a.targetX, z: a.targetZ, rotY: a.targetRotY }));

    coffeeBarAgents.forEach((a, i) => {
      a.targetX = -20 + (i % 4) * 1.8;
      a.targetZ = 11 + Math.floor(i / 4) * 1.5;
      a.targetRotY = 0;
      a.bubbleText = 'Discussing sprint roadmap over coffee...';
      a.isTyping = false;
    });
    await new Promise(r => setTimeout(r, 3500));

    // Phase 2: All-Hands Emergency Release Meeting in Conference Room
    globalBubble = 'PHASE 2: All-Hands Emergency Release Meeting in Conference Room';
    coffeeBarAgents.forEach((a, i) => {
      a.bubbleText = 'Heading to Conference Room for release...';
      a.targetX = 10 + (i % 3) * 2;
      a.targetZ = 6 + Math.floor(i / 3) * 1.8;
      a.targetRotY = Math.PI / 2;
    });
    await new Promise(r => setTimeout(r, 4000));

    // Phase 3: CEO Executive Review & Release Signoff
    globalBubble = 'PHASE 3: Executive Board Approval in CEO Suite';
    const leadAgent = agents3D[2] || agents3D[0];
    const originalLeadPos = { x: leadAgent.targetX, z: leadAgent.targetZ, rotY: leadAgent.targetRotY };
    leadAgent.targetX = 13;
    leadAgent.targetZ = -8;
    leadAgent.targetRotY = 0;
    leadAgent.bubbleText = 'Pitching production release to CEO...';
    await new Promise(r => setTimeout(r, 3500));

    // Phase 4: Full Production Deployment - Coding Frenzy at Workstations
    globalBubble = 'PHASE 4: Production Sprint Deployment! All 35 Agents Coding';
    coffeeBarAgents.forEach((a, i) => {
      a.targetX = originalPositions[i].x;
      a.targetZ = originalPositions[i].z;
      a.targetRotY = originalPositions[i].rotY || Math.PI;
      a.bubbleText = 'Deploying production code!';
      a.isTyping = true;
    });

    leadAgent.targetX = originalLeadPos.x;
    leadAgent.targetZ = originalLeadPos.z;
    leadAgent.targetRotY = originalLeadPos.rotY || Math.PI;
    leadAgent.bubbleText = 'Reviewing PRs & CI/CD logs';
    leadAgent.isTyping = true;

    agents3D.forEach(a => {
      a.isTyping = true;
    });

    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'SUCCESS', message: `Production release deployed! All 35 workstations active.` }];
    await new Promise(r => setTimeout(r, 4000));

    agents3D.forEach(a => {
      a.bubbleText = '';
    });
    globalBubble = 'VIRTUAL OFFICE: Production Release Complete! Team Working on Workstations.';
    isSimulating = false;
  }

  function setCameraView(preset: 'floorplan' | 'staff' | 'ceo' | 'conference' | 'reception') {
    if (!camera || !controls) return;
    if (preset === 'floorplan') {
      camera.position.set(36, 42, 36);
      controls.target.set(0, 0, 0);
    } else if (preset === 'staff') {
      camera.position.set(-10, 22, 12);
      controls.target.set(-10, 1, 0);
    } else if (preset === 'ceo') {
      camera.position.set(14, 18, -4);
      controls.target.set(12, 1, -10);
    } else if (preset === 'conference') {
      camera.position.set(14, 18, 14);
      controls.target.set(12, 1, 8);
    } else if (preset === 'reception') {
      camera.position.set(0, 16, 22);
      controls.target.set(0, 1, 14);
    }
    controls.update();
  }

  function init3D() {
    if (!container) return;

    scene = new THREE.Scene();
    scene.background = new THREE.Color(0x0f172a); // Slate Canvas

    const aspect = container.clientWidth / container.clientHeight;
    const d = 26;
    camera = new THREE.OrthographicCamera(-d * aspect, d * aspect, d, -d, 1, 1000);
    camera.position.set(36, 42, 36);
    camera.lookAt(0, 0, 0);

    const ambientLight = new THREE.AmbientLight(0xffedd5, 0.9);
    scene.add(ambientLight);

    const dirLight = new THREE.DirectionalLight(0xfff7ed, 1.35);
    dirLight.position.set(32, 58, 38);
    dirLight.castShadow = true;
    dirLight.shadow.camera.left = -34;
    dirLight.shadow.camera.right = 34;
    dirLight.shadow.camera.top = 34;
    dirLight.shadow.camera.bottom = -34;
    dirLight.shadow.mapSize.width = 2048;
    dirLight.shadow.mapSize.height = 2048;
    scene.add(dirLight);

    const ceoSpot = new THREE.PointLight(0xfef08a, 1.8, 22);
    ceoSpot.position.set(13, 6.5, -9);
    scene.add(ceoSpot);

    const confSpot = new THREE.PointLight(0xbae6fd, 1.8, 22);
    confSpot.position.set(13, 6.5, 8);
    scene.add(confSpot);

    const recepSpot = new THREE.PointLight(0xfed7aa, 1.8, 22);
    recepSpot.position.set(0, 6.5, 14);
    scene.add(recepSpot);

    buildArchitecturalStructure();
    rebuildOfficeLayout();

    renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setSize(container.clientWidth, container.clientHeight);
    renderer.setPixelRatio(window.devicePixelRatio);
    renderer.shadowMap.enabled = true;
    renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    container.appendChild(renderer.domElement);

    controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.dampingFactor = 0.05;

    animate();
  }

  function buildArchitecturalStructure() {
    const wallMat = new THREE.MeshStandardMaterial({ color: 0xf8fafc, roughness: 0.85 });
    const woodWallMat = new THREE.MeshStandardMaterial({ color: 0x3b170b, roughness: 0.5 });
    const glassMat = new THREE.MeshStandardMaterial({ color: 0x38bdf8, transparent: true, opacity: 0.25 });

    // Photorealistic Floor Materials
    const staffFloorMat = new THREE.MeshStandardMaterial({ color: 0x271c19, roughness: 0.25, metalness: 0.15 });
    const staffFloor = new THREE.Mesh(new THREE.PlaneGeometry(30, 30), staffFloorMat);
    staffFloor.rotation.x = -Math.PI / 2;
    staffFloor.position.set(-10, 0.01, 0);
    staffFloor.receiveShadow = true;

    const ceoFloorMat = new THREE.MeshStandardMaterial({ color: 0x4c0519, roughness: 0.85 });
    const ceoFloor = new THREE.Mesh(new THREE.PlaneGeometry(18, 15), ceoFloorMat);
    ceoFloor.rotation.x = -Math.PI / 2;
    ceoFloor.position.set(13, 0.02, -8);
    ceoFloor.receiveShadow = true;

    const confFloorMat = new THREE.MeshStandardMaterial({ color: 0x4a044e, roughness: 0.85 });
    const confFloor = new THREE.Mesh(new THREE.PlaneGeometry(18, 15), confFloorMat);
    confFloor.rotation.x = -Math.PI / 2;
    confFloor.position.set(13, 0.02, 8);
    confFloor.receiveShadow = true;

    const recepFloorMat = new THREE.MeshStandardMaterial({ color: 0xecfeff, roughness: 0.12, metalness: 0.45 });
    const recepFloor = new THREE.Mesh(new THREE.PlaneGeometry(14, 10), recepFloorMat);
    recepFloor.rotation.x = -Math.PI / 2;
    recepFloor.position.set(0, 0.03, 14);
    recepFloor.receiveShadow = true;

    scene.add(staffFloor, ceoFloor, confFloor, recepFloor);

    function addWall(x: number, z: number, w: number, d: number, h = 5.0, mat = wallMat) {
      const mesh = new THREE.Mesh(new THREE.BoxGeometry(w, h, d), mat);
      mesh.position.set(x, h / 2, z);
      mesh.castShadow = true;
      mesh.receiveShadow = true;
      scene.add(mesh);
    }

    addWall(-25, 0, 0.5, 30);
    addWall(0, -15, 50, 0.5);
    addWall(22, 0, 0.5, 30);
    addWall(-14, 15, 22, 0.5);
    addWall(14, 15, 16, 0.5);

    addWall(4, -3, 0.5, 24);
    addWall(13, 0, 18, 0.5);

    const glassDoor = new THREE.Mesh(new THREE.BoxGeometry(5, 4.5, 0.1), glassMat);
    glassDoor.position.set(0, 2.25, 15);
    scene.add(glassDoor);

    addWall(13, -14.6, 17.5, 0.4, 5.0, woodWallMat);

    for (let i = 0; i < 5; i++) {
      const win = new THREE.Mesh(new THREE.BoxGeometry(0.1, 3.0, 4), glassMat);
      win.position.set(-24.9, 3.0, -10 + i * 5.5);
      scene.add(win);
    }

    buildCoffeeBar();
    buildDecorations();
  }

  function buildCoffeeBar() {
    const group = new THREE.Group();
    group.position.set(-22, 0, 12);

    const counter = new THREE.Mesh(new THREE.BoxGeometry(3.8, 1.25, 1.5), new THREE.MeshStandardMaterial({ color: 0x18181b, roughness: 0.3 }));
    counter.position.y = 0.62;
    counter.castShadow = true;

    const coffeeMachine = new THREE.Mesh(new THREE.BoxGeometry(0.9, 0.85, 0.65), new THREE.MeshStandardMaterial({ color: 0xd97706, metalness: 0.8, roughness: 0.2 }));
    coffeeMachine.position.set(-0.9, 1.65, 0);

    const dispenser = new THREE.Mesh(new THREE.CylinderGeometry(0.32, 0.32, 1.5, 16), new THREE.MeshStandardMaterial({ color: 0x38bdf8, transparent: true, opacity: 0.65 }));
    dispenser.position.set(0.9, 2.0, 0);

    group.add(counter, coffeeMachine, dispenser);
    scene.add(group);
    deskObjects.push(group);
  }

  function buildDecorations() {
    const plantPositions = [
      { x: -23, z: -13 }, { x: 2, z: -13 }, { x: 20, z: -13 },
      { x: -23, z: 13 },  { x: 20, z: 13 }
    ];

    plantPositions.forEach(p => {
      const plantGroup = new THREE.Group();
      plantGroup.position.set(p.x, 0, p.z);

      const pot = new THREE.Mesh(new THREE.CylinderGeometry(0.45, 0.35, 0.8, 16), new THREE.MeshStandardMaterial({ color: 0xf8fafc, roughness: 0.3 }));
      pot.position.y = 0.4;
      pot.castShadow = true;

      const bush = new THREE.Mesh(new THREE.DodecahedronGeometry(0.75, 1), new THREE.MeshStandardMaterial({ color: 0x15803d, roughness: 0.8 }));
      bush.position.y = 1.3;
      bush.castShadow = true;

      plantGroup.add(pot, bush);
      scene.add(plantGroup);
      deskObjects.push(plantGroup);
    });

    const tvGroup = new THREE.Group();
    tvGroup.position.set(21.7, 3.0, 8);

    const tvFrame = new THREE.Mesh(new THREE.BoxGeometry(0.1, 2.2, 4.0), new THREE.MeshStandardMaterial({ color: 0x090d16 }));
    const tvScreen = new THREE.Mesh(new THREE.PlaneGeometry(3.9, 2.1), new THREE.MeshBasicMaterial({ color: 0x0284c7 }));
    tvScreen.rotation.y = -Math.PI / 2;
    tvScreen.position.x = -0.051;
    tvGroup.add(tvFrame, tvScreen);

    scene.add(tvGroup);
    deskObjects.push(tvGroup);
  }

  function rebuildOfficeLayout() {
    deskObjects.forEach(obj => scene.remove(obj));
    deskObjects = [];
    agents3D = [];
    uiAgents = [];

    buildReceptionLobby();
    buildCEOSuite();
    buildConferenceRoom();

    const colors = [0x0284c7, 0x7c3aed, 0xd97706, 0x059669, 0xdc2626, 0xdb2777, 0x4f46e5, 0x0d9488, 0x0891b2];
    staffPodPositions.forEach((pos, idx) => {
      createStaffDualMonitorDesk(pos.x, pos.z, colors[idx % colors.length]);
    });

    if (!isAllSeatsPopulated) {
      assignAgentsToSeats();
    }
  }

  function assignAgentsToSeats() {
    let agentList = currentAgentList || [];
    const colors = [0x0284c7, 0x7c3aed, 0xd97706, 0x059669, 0xdc2626, 0xdb2777, 0x4f46e5, 0x0d9488, 0x0891b2];

    agentList.forEach((agent, index) => {
      const color = colors[index % colors.length];
      const nameLower = (agent.name || '').toLowerCase();

      let seatX = -9;
      let seatZ = 0;
      let rotY = Math.PI;

      if (nameLower.includes('architect') || nameLower.includes('tech lead')) {
        seatX = 13;
        seatZ = -9.4;
        rotY = Math.PI;
      } else if (nameLower.includes('product manager') || nameLower.includes('project manager')) {
        seatX = 13;
        seatZ = 6;
        rotY = Math.PI / 2;
      } else {
        const pod = staffPodPositions[index % staffPodPositions.length];
        seatX = pod.x;
        seatZ = pod.z + 0.9;
        rotY = Math.PI;
      }

      createCharacterAvatar(agent.name || `Agent ${index + 1}`, color, seatX, seatZ, rotY);
    });
  }

  function buildReceptionLobby() {
    const group = new THREE.Group();
    group.position.set(0, 0, 14);

    const desk = new THREE.Mesh(new THREE.BoxGeometry(5.5, 1.4, 2.0), new THREE.MeshStandardMaterial({ color: 0xf8fafc, roughness: 0.2 }));
    desk.position.y = 0.7;
    desk.castShadow = true;

    const trim = new THREE.Mesh(new THREE.BoxGeometry(5.55, 0.35, 0.1), new THREE.MeshStandardMaterial({ color: 0x3b170b }));
    trim.position.set(0, 0.7, 1.0);

    const chair = createRGBGamingChair(0x38bdf8);
    chair.position.set(0, 0, -0.8);
    chair.rotation.y = Math.PI;

    group.add(desk, trim, chair);
    scene.add(group);
    deskObjects.push(group);
  }

  function buildCEOSuite() {
    const group = new THREE.Group();
    group.position.set(13, 0, -8);

    const deskMat = new THREE.MeshStandardMaterial({ color: 0x3b170b, roughness: 0.4 });
    const top = new THREE.Mesh(new THREE.BoxGeometry(5.5, 0.16, 2.8), deskMat);
    top.position.set(0, 1.4, 0);
    top.castShadow = true;

    const returnDesk = new THREE.Mesh(new THREE.BoxGeometry(2.4, 0.16, 2.0), deskMat);
    returnDesk.position.set(2.2, 1.4, 1.4);
    returnDesk.castShadow = true;

    const monGroup = createDualMonitors(0xd97706);
    monGroup.position.set(0, 1.45, -0.4);

    // CEO RGB Gaming Chair facing North towards desk
    const ceoChair = createRGBGamingChair(0xd97706);
    ceoChair.position.set(0, 0, -1.4);
    ceoChair.rotation.y = Math.PI;

    const sofa = new THREE.Mesh(new THREE.BoxGeometry(4.0, 0.9, 1.3), new THREE.MeshStandardMaterial({ color: 0x18181b }));
    sofa.position.set(-5.0, 0.45, 1.2);
    sofa.rotation.y = Math.PI / 2;

    const coffeeTable = new THREE.Mesh(new THREE.BoxGeometry(2.0, 0.4, 1.2), deskMat);
    coffeeTable.position.set(-3.2, 0.2, 1.2);

    group.add(top, returnDesk, monGroup, ceoChair, sofa, coffeeTable);
    scene.add(group);
    deskObjects.push(group);
  }

  function buildConferenceRoom() {
    const group = new THREE.Group();
    group.position.set(13, 0, 8);

    const tableGeo = new THREE.CylinderGeometry(3.0, 3.0, 0.16, 32);
    tableGeo.scale(2.4, 1, 1);
    const table = new THREE.Mesh(tableGeo, new THREE.MeshStandardMaterial({ color: 0x3b170b, roughness: 0.3, metalness: 0.3 }));
    table.position.y = 1.35;
    table.castShadow = true;
    group.add(table);

    for (let i = 0; i < 12; i++) {
      const angle = (i / 12) * Math.PI * 2;
      const cx = Math.cos(angle) * 5.0;
      const cz = Math.sin(angle) * 2.8;
      const chair = new THREE.Mesh(new THREE.BoxGeometry(0.7, 1.0, 0.7), new THREE.MeshStandardMaterial({ color: 0x090d16 }));
      chair.position.set(cx, 0.5, cz);
      chair.rotation.y = -angle + Math.PI / 2;
      chair.castShadow = true;
      group.add(chair);
    }

    scene.add(group);
    deskObjects.push(group);
  }

  function createDualMonitors(color: number) {
    const group = new THREE.Group();
    const monitorGeo = new THREE.BoxGeometry(1.5, 0.85, 0.06);
    const monitorMat = new THREE.MeshStandardMaterial({ color: 0x090d16, roughness: 0.3 });
    const screenGeo = new THREE.PlaneGeometry(1.4, 0.75);
    const screenMat = new THREE.MeshBasicMaterial({ color: 0x0f172a });

    const codeLinesGroup = new THREE.Group();
    for (let i = 0; i < 5; i++) {
      const lineGeo = new THREE.PlaneGeometry(0.8 + Math.random() * 0.4, 0.05);
      const lineMat = new THREE.MeshBasicMaterial({ color: i % 2 === 0 ? color : 0x38bdf8 });
      const line = new THREE.Mesh(lineGeo, lineMat);
      line.position.set(-0.2, 0.2 - i * 0.12, 0.001);
      codeLinesGroup.add(line);
    }

    const m1 = new THREE.Group();
    const mon1 = new THREE.Mesh(monitorGeo, monitorMat);
    const scr1 = new THREE.Mesh(screenGeo, screenMat);
    scr1.position.z = 0.031;
    scr1.add(codeLinesGroup.clone());
    m1.add(mon1, scr1);
    m1.position.set(-0.85, 0.45, 0);
    m1.rotation.y = Math.PI / 9;

    const m2 = new THREE.Group();
    const mon2 = new THREE.Mesh(monitorGeo, monitorMat);
    const scr2 = new THREE.Mesh(screenGeo, screenMat);
    scr2.position.z = 0.031;
    scr2.add(codeLinesGroup.clone());
    m2.add(mon2, scr2);
    m2.position.set(0.85, 0.45, 0);
    m2.rotation.y = -Math.PI / 9;

    group.add(m1, m2);
    return group;
  }

  function createRGBGamingChair(color: number) {
    const chairGroup = new THREE.Group();

    const backGeo = new THREE.BoxGeometry(0.85, 1.25, 0.12);
    const chairMat = new THREE.MeshStandardMaterial({ color: 0x18181b, roughness: 0.5 });
    const back = new THREE.Mesh(backGeo, chairMat);
    back.position.set(0, 1.2, 0);
    back.castShadow = true;

    const bolsterGeo = new THREE.BoxGeometry(0.15, 1.1, 0.22);
    const bolsterMat = new THREE.MeshStandardMaterial({ color: color, roughness: 0.4 });
    const leftBolster = new THREE.Mesh(bolsterGeo, bolsterMat);
    leftBolster.position.set(-0.45, 1.2, 0.05);
    const rightBolster = new THREE.Mesh(bolsterGeo, bolsterMat);
    rightBolster.position.set(0.45, 1.2, 0.05);

    const seatGeo = new THREE.BoxGeometry(0.85, 0.14, 0.85);
    const seat = new THREE.Mesh(seatGeo, chairMat);
    seat.position.y = 0.65;
    seat.castShadow = true;

    const rgbEdgeGeo = new THREE.BoxGeometry(0.87, 1.27, 0.02);
    const rgbEdgeMat = new THREE.MeshBasicMaterial({ color: color });
    const rgbEdge = new THREE.Mesh(rgbEdgeGeo, rgbEdgeMat);
    rgbEdge.position.set(0, 1.2, -0.06);

    const poleMat = new THREE.MeshStandardMaterial({ color: 0x64748b, metalness: 0.9 });
    const pole = new THREE.Mesh(new THREE.CylinderGeometry(0.06, 0.06, 0.5), poleMat);
    pole.position.y = 0.35;

    const base = new THREE.Mesh(new THREE.CylinderGeometry(0.45, 0.45, 0.06, 5), poleMat);
    base.position.y = 0.1;

    chairGroup.add(back, leftBolster, rightBolster, seat, rgbEdge, pole, base);
    return chairGroup;
  }

  function createStaffDualMonitorDesk(x: number, z: number, color: number) {
    const deskGroup = new THREE.Group();
    deskGroup.position.set(x, 0, z);

    // Desktop Surface
    const top = new THREE.Mesh(new THREE.BoxGeometry(3.6, 0.1, 1.8), new THREE.MeshStandardMaterial({ color: 0x18181b, roughness: 0.4 }));
    top.position.y = 1.3;
    top.castShadow = true;

    const edge = new THREE.Mesh(new THREE.BoxGeometry(3.65, 0.04, 1.85), new THREE.MeshBasicMaterial({ color: color }));
    edge.position.y = 1.3;

    const partition = new THREE.Mesh(new THREE.BoxGeometry(3.6, 0.9, 0.04), new THREE.MeshStandardMaterial({ color: 0x38bdf8, transparent: true, opacity: 0.22 }));
    partition.position.set(0, 1.75, -0.9);

    // Dual Curved Monitors
    const dualMonitors = createDualMonitors(color);
    dualMonitors.position.set(0, 1.35, -0.4);

    // 1. Extended RGB Desk Mat / Mousepad
    const deskPadGeo = new THREE.BoxGeometry(2.4, 0.015, 1.0);
    const deskPadMat = new THREE.MeshStandardMaterial({ color: 0x090d16, roughness: 0.6 });
    const deskPad = new THREE.Mesh(deskPadGeo, deskPadMat);
    deskPad.position.set(0, 1.358, -0.05);

    const padEdge = new THREE.Mesh(new THREE.BoxGeometry(2.44, 0.01, 1.04), new THREE.MeshBasicMaterial({ color: color }));
    padEdge.position.set(0, 1.355, -0.05);

    // 2. Mechanical RGB Keyboard
    const kbGeo = new THREE.BoxGeometry(1.0, 0.04, 0.36);
    const kbMat = new THREE.MeshStandardMaterial({ color: 0x18181b, roughness: 0.3 });
    const keyboard = new THREE.Mesh(kbGeo, kbMat);
    keyboard.position.set(-0.2, 1.38, -0.05);

    const keysLight = new THREE.Mesh(new THREE.PlaneGeometry(0.92, 0.28), new THREE.MeshBasicMaterial({ color: color }));
    keysLight.rotation.x = -Math.PI / 2;
    keysLight.position.set(-0.2, 1.401, -0.05);

    // 3. Ergonomic Wireless Gaming Mouse
    const mouseGeo = new THREE.BoxGeometry(0.14, 0.04, 0.24);
    const mouseMat = new THREE.MeshStandardMaterial({ color: 0x0f172a, roughness: 0.3 });
    const mouse = new THREE.Mesh(mouseGeo, mouseMat);
    mouse.position.set(0.7, 1.38, -0.05);

    // RGB Gaming Chair facing North towards desk
    const gamingChair = createRGBGamingChair(color);
    gamingChair.position.set(0, 0, 0.9);
    gamingChair.rotation.y = Math.PI;

    deskGroup.add(top, edge, partition, dualMonitors, deskPad, padEdge, keyboard, keysLight, mouse, gamingChair);
    scene.add(deskGroup);
    deskObjects.push(deskGroup);
  }

  function createCharacterAvatar(name: string, color: number, x: number, z: number, rotY = 0) {
    const group = new THREE.Group();
    group.position.set(x, 0, z);
    group.rotation.y = rotY;

    const skinTones = [0xffdbac, 0xf1c27d, 0xe0ac69, 0xc58c85];
    const skinColor = skinTones[Math.abs(Math.floor(x + z)) % skinTones.length];
    const skinMat = new THREE.MeshStandardMaterial({ color: skinColor, roughness: 0.7 });
    const shirtMat = new THREE.MeshStandardMaterial({ color: color, roughness: 0.5 });
    const pantsMat = new THREE.MeshStandardMaterial({ color: 0x1e293b, roughness: 0.8 });

    const headGroup = new THREE.Group();
    headGroup.position.y = 1.5;

    const head = new THREE.Mesh(new THREE.SphereGeometry(0.2, 16, 16), skinMat);
    const hair = new THREE.Mesh(new THREE.SphereGeometry(0.21, 16, 16, 0, Math.PI * 2, 0, Math.PI / 2.2), new THREE.MeshStandardMaterial({ color: 0x271c19 }));
    hair.position.y = 0.02;

    const headsetBand = new THREE.Mesh(new THREE.TorusGeometry(0.22, 0.02, 8, 24, Math.PI), new THREE.MeshStandardMaterial({ color: 0x090d16 }));
    headsetBand.rotation.x = Math.PI / 2;
    headsetBand.position.y = 0.06;

    headGroup.add(head, hair, headsetBand);
    group.add(headGroup);

    const torso = new THREE.Mesh(new THREE.BoxGeometry(0.5, 0.6, 0.3), shirtMat);
    torso.position.y = 1.0;
    group.add(torso);

    const leftLegGroup = new THREE.Group();
    leftLegGroup.position.set(-0.15, 0.7, 0);
    const leftLeg = new THREE.Mesh(new THREE.BoxGeometry(0.2, 0.55, 0.2), pantsMat);
    leftLeg.position.y = -0.27;
    leftLegGroup.add(leftLeg);

    const rightLegGroup = new THREE.Group();
    rightLegGroup.position.set(0.15, 0.7, 0);
    const rightLeg = new THREE.Mesh(new THREE.BoxGeometry(0.2, 0.55, 0.2), pantsMat);
    rightLeg.position.y = -0.27;
    rightLegGroup.add(rightLeg);

    const leftArmGroup = new THREE.Group();
    leftArmGroup.position.set(-0.35, 1.25, 0);
    const leftArm = new THREE.Mesh(new THREE.BoxGeometry(0.16, 0.5, 0.16), shirtMat);
    leftArm.position.y = -0.25;
    leftArmGroup.add(leftArm);

    const rightArmGroup = new THREE.Group();
    rightArmGroup.position.set(0.35, 1.25, 0);
    const rightArm = new THREE.Mesh(new THREE.BoxGeometry(0.16, 0.5, 0.16), shirtMat);
    rightArm.position.y = -0.25;
    rightArmGroup.add(rightArm);

    group.add(leftLegGroup, rightLegGroup, leftArmGroup, rightArmGroup);

    scene.add(group);
    deskObjects.push(group);

    agents3D.push({
      mesh: group,
      name,
      targetX: x,
      targetZ: z,
      targetRotY: rotY,
      bubbleText: '',
      isTyping: false,
      parts: { leftLeg: leftLegGroup, rightLeg: rightLegGroup, leftArm: leftArmGroup, rightArm: rightArmGroup }
    });
  }

  function animate() {
    animId = requestAnimationFrame(animate);

    const time = Date.now() * 0.003;
    agents3D.forEach((a, idx) => {
      const dx = a.targetX - a.mesh.position.x;
      const dz = a.targetZ - a.mesh.position.z;
      const isMoving = Math.abs(dx) > 0.01 || Math.abs(dz) > 0.01;

      if (isMoving) {
        a.mesh.position.x += dx * 0.05;
        a.mesh.position.z += dz * 0.05;

        const targetAngle = Math.atan2(dx, dz);
        let diff = targetAngle - a.mesh.rotation.y;
        while (diff < -Math.PI) diff += Math.PI * 2;
        while (diff > Math.PI) diff -= Math.PI * 2;
        a.mesh.rotation.y += diff * 0.1;

        a.mesh.position.y = Math.abs(Math.sin(time * 12 * 2)) * 0.05;

        const walkAngle = Math.sin(time * 12) * 0.7;
        if (a.parts) {
          a.parts.leftLeg.rotation.x = walkAngle;
          a.parts.rightLeg.rotation.x = -walkAngle;
          a.parts.leftArm.rotation.x = -walkAngle;
          a.parts.rightArm.rotation.x = walkAngle;
        }
      } else {
        // Seated comfortably ON TOP of chair cushion (y = 0.05) facing desk (targetRotY)
        const targetRotY = a.targetRotY !== undefined ? a.targetRotY : Math.PI;
        let diff = targetRotY - a.mesh.rotation.y;
        while (diff < -Math.PI) diff += Math.PI * 2;
        while (diff > Math.PI) diff -= Math.PI * 2;
        a.mesh.rotation.y += diff * 0.1;

        a.mesh.position.y = 0.05;

        if (a.parts) {
          a.parts.leftLeg.rotation.x = -Math.PI / 2;
          a.parts.rightLeg.rotation.x = -Math.PI / 2;

          const typingSpeed = a.isTyping ? 18 : 3;
          const typingAmp = a.isTyping ? 0.25 : 0.05;
          a.parts.leftArm.rotation.x = -Math.PI / 3 + Math.sin(time * typingSpeed) * typingAmp;
          a.parts.rightArm.rotation.x = -Math.PI / 3 + Math.cos(time * typingSpeed) * typingAmp;
        }
      }

      if (!uiAgents[idx]) {
        uiAgents[idx] = { id: idx, name: a.name, text: '', x: 0, y: 0, visible: false };
      }

      if (a.bubbleText) {
        const vector = new THREE.Vector3();
        vector.setFromMatrixPosition(a.mesh.matrixWorld);
        vector.y += 2.2;
        vector.project(camera);

        uiAgents[idx].x = (vector.x * .5 + .5) * container.clientWidth;
        uiAgents[idx].y = (vector.y * -.5 + .5) * container.clientHeight;
        uiAgents[idx].text = a.bubbleText;
        uiAgents[idx].visible = true;
      } else {
        uiAgents[idx].visible = false;
      }
    });

    uiAgents = uiAgents;

    if (controls) controls.update();
    renderer.render(scene, camera);
  }

  function handleResize() {
    if (!container || !renderer || !camera) return;
    const aspect = container.clientWidth / container.clientHeight;
    const d = 26;
    camera.left = -d * aspect;
    camera.right = d * aspect;
    camera.top = d;
    camera.bottom = -d;
    camera.updateProjectionMatrix();
    renderer.setSize(container.clientWidth, container.clientHeight);
  }
</script>

<div class="relative w-full h-[calc(100vh-100px)] overflow-hidden rounded-xl border border-outline-variant bg-surface shadow-sm">
  <!-- 3D Canvas Viewport -->
  <div bind:this={container} class="w-full h-full"></div>

  <!-- Global Status Bubble Overlay -->
  <div class="absolute top-6 left-1/2 -translate-x-1/2 bg-surface-container-lowest border border-outline-variant px-4 py-2 rounded-xl text-xs font-semibold shadow-md animate-bounce flex items-center gap-2">
    <span class="w-2 h-2 rounded-full {isSimulating ? 'bg-primary animate-ping' : 'bg-emerald-500'}"></span>
    <span class="text-primary font-bold">{isSimulating ? 'SIMULATING:' : 'REALISTIC OFFICE:'}</span> {globalBubble}
  </div>

  <!-- Camera Presets Bar -->
  <div class="absolute top-6 left-6 bg-surface-container-lowest border border-outline-variant p-1.5 rounded-xl shadow-md flex gap-1 text-xs">
    <button on:click={() => setCameraView('floorplan')} class="px-3 py-1.5 rounded-lg font-bold hover:bg-surface-container-high transition-all text-on-surface flex items-center gap-1 cursor-pointer">
      <span class="material-symbols-outlined text-sm text-primary">view_in_ar</span> Floorplan (35 Seats)
    </button>
    <button on:click={() => setCameraView('staff')} class="px-3 py-1.5 rounded-lg font-bold hover:bg-surface-container-high transition-all text-on-surface flex items-center gap-1 cursor-pointer">
      <span class="material-symbols-outlined text-sm text-primary">groups</span> Staff Zone (18 Pods)
    </button>
    <button on:click={() => setCameraView('ceo')} class="px-3 py-1.5 rounded-lg font-bold hover:bg-surface-container-high transition-all text-on-surface flex items-center gap-1 cursor-pointer">
      <span class="material-symbols-outlined text-sm text-primary">badge</span> CEO Suite
    </button>
    <button on:click={() => setCameraView('conference')} class="px-3 py-1.5 rounded-lg font-bold hover:bg-surface-container-high transition-all text-on-surface flex items-center gap-1 cursor-pointer">
      <span class="material-symbols-outlined text-sm text-primary">meeting_room</span> Conference (12 Seats)
    </button>
    <button on:click={() => setCameraView('reception')} class="px-3 py-1.5 rounded-lg font-bold hover:bg-surface-container-high transition-all text-on-surface flex items-center gap-1 cursor-pointer">
      <span class="material-symbols-outlined text-sm text-primary">door_front</span> Reception
    </button>
  </div>

  <!-- Agent Speech Bubbles -->
  {#each uiAgents as uiAgent}
    {#if uiAgent.visible}
      <div 
        class="absolute -translate-x-1/2 -translate-y-full bg-surface-container-lowest border border-outline-variant px-3 py-1.5 rounded-xl text-[11px] font-semibold shadow-xl whitespace-nowrap pointer-events-none transition-all duration-75 text-on-surface"
        style="left: {uiAgent.x}px; top: {uiAgent.y}px;"
      >
        <div class="absolute -bottom-1.5 left-1/2 -translate-x-1/2 w-3 h-3 bg-surface-container-lowest border-b border-r border-outline-variant rotate-45"></div>
        <span class="text-primary font-bold">{uiAgent.name}:</span> {uiAgent.text}
      </div>
    {/if}
  {/each}

  <!-- Control Panel with 2 New Interactive Simulation Buttons -->
  <div class="absolute top-6 right-6 bg-surface-container-lowest border border-outline-variant px-4 py-3 rounded-xl shadow-md space-y-3 w-72">
    <h3 class="text-xs font-bold text-on-surface uppercase tracking-wider flex items-center gap-1">
      <span class="material-symbols-outlined text-sm text-primary">sports_esports</span> Interactive Simulation
    </h3>
    <p class="text-[10px] text-on-surface-variant leading-tight">Photorealistic PBR Office Architecture with 35 seats, Dual Monitors, RGB Keyboards & Gaming Chairs.</p>
    
    <div class="space-y-2">
      <!-- Button 1: Populate All 35 Seats -->
      <button 
        on:click={handlePopulateAllSeats}
        class="w-full bg-surface-container-high text-on-surface border border-outline-variant px-3 py-2 rounded-lg text-xs font-bold hover:bg-surface-container-highest flex items-center justify-center gap-2 transition-all cursor-pointer">
        <span class="material-symbols-outlined text-sm text-primary">person_add</span> 👥 Gắn Agent vào Tất Cả 35 Vị Trí
      </button>

      <!-- Button 2: Run Full Office Corporate Scenario -->
      <button 
        on:click={handleRunFullOfficeScenario} 
        disabled={isSimulating}
        class="w-full bg-primary text-on-primary px-3 py-2 rounded-lg text-xs font-bold hover:brightness-110 flex items-center justify-center gap-2 transition-all disabled:opacity-50 cursor-pointer shadow-xs">
        <span class="material-symbols-outlined text-sm">{isSimulating ? 'sync' : 'movie'}</span> 🎬 Chạy Kịch Bản Giả Lập Toàn Văn Phòng
      </button>
    </div>
  </div>

  <!-- Bottom Real-time Console Log Overlay -->
  <div class="absolute bottom-6 left-6 w-96 bg-surface-container-lowest border border-outline-variant rounded-xl p-3 shadow-xl space-y-2">
    <div class="flex items-center justify-between border-b border-outline-variant pb-1">
      <span class="text-[10px] font-bold uppercase text-on-surface-variant flex items-center gap-1">
        <span class="material-symbols-outlined text-xs">terminal</span> Corporate Scenario Logs
      </span>
      <span class="text-[9px] font-bold uppercase text-primary bg-primary-container px-2 py-0.5 rounded">35 SEATS ACTIVE</span>
    </div>
    <div class="font-mono text-[11px] space-y-1 max-h-28 overflow-y-auto">
      {#each $logs.slice(-4) as log}
        <div class="flex gap-2">
          <span class="text-secondary font-bold">[{log.level}]</span>
          <span class="text-on-surface line-clamp-1">{log.message}</span>
        </div>
      {:else}
        <div class="text-on-surface-variant italic text-[10px]">Photorealistic 35-seat office active. Click simulation buttons to test...</div>
      {/each}
    </div>
  </div>
</div>
