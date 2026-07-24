<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as THREE from 'three';
  import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
  import { logs } from '../../lib/stores/appState';

  let container: HTMLDivElement;
  let scene: THREE.Scene;
  let camera: THREE.OrthographicCamera;
  let renderer: THREE.WebGLRenderer;
  let animId: number;
  let controls: OrbitControls;

  let isSimulating = false;
  let globalBubble = 'THINKING: Analyzing repo architecture...';

  let agents3D: { mesh: THREE.Group; name: string; targetX: number; targetZ: number; bubbleText: string; parts?: any }[] = [];
  let uiAgents: { id: number; name: string; text: string; x: number; y: number; visible: boolean }[] = [];

  onMount(() => {
    init3D();
    window.addEventListener('resize', handleResize);
  });

  onDestroy(() => {
    if (animId) cancelAnimationFrame(animId);
    window.removeEventListener('resize', handleResize);
  });

  async function handleTestRun() {
    if (isSimulating) return;
    isSimulating = true;

    const chief = agents3D[0];
    const arch = agents3D[1];
    const coder = agents3D[2];

    const chiefStart = { x: chief.targetX, z: chief.targetZ };
    const archStart = { x: arch.targetX, z: arch.targetZ };
    const coderStart = { x: coder.targetX, z: coder.targetZ };
    
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'INFO', message: 'Initiated 3D Simulation Test Run' }];
    
    globalBubble = 'Architect: Planning database schema...';
    arch.bubbleText = 'Planning schema...';
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'SYSTEM', message: 'Architect agent is planning database schema...' }];
    await new Promise(r => setTimeout(r, 2000));
    
    // Move Arch to Coder
    arch.targetX = coderStart.x - 1.5;
    arch.targetZ = coderStart.z;
    globalBubble = 'Architect: Handing off spec to Coder...';
    arch.bubbleText = 'Hey Coder, here are the specs!';
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'SYSTEM', message: 'Architect -> Coder: Handoff specs' }];
    await new Promise(r => setTimeout(r, 2000));
    
    // Arch returns
    arch.targetX = archStart.x;
    arch.targetZ = archStart.z;
    arch.bubbleText = '';

    globalBubble = 'Coder: Writing boilerplate code...';
    coder.bubbleText = 'Writing boilerplate code...';
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'DEV', message: 'Lead Coder agent is generating boilerplate code...' }];
    await new Promise(r => setTimeout(r, 2000));
    
    // Move Coder to Chief
    coder.targetX = chiefStart.x + 1.5;
    coder.targetZ = chiefStart.z;
    globalBubble = 'Coder: Handing off code for review...';
    coder.bubbleText = 'Code ready for review, Chief!';
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'DEV', message: 'Coder -> Chief AI: Code Review Request' }];
    await new Promise(r => setTimeout(r, 2000));
    
    // Coder returns
    coder.targetX = coderStart.x;
    coder.targetZ = coderStart.z;
    coder.bubbleText = '';

    globalBubble = 'QA: Running unit tests...';
    chief.bubbleText = 'Running unit tests...';
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'QA', message: 'Chief AI is running unit tests...' }];
    await new Promise(r => setTimeout(r, 2000));
    
    globalBubble = 'DONE: Simulation finished!';
    chief.bubbleText = 'All tests passed!';
    $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'SUCCESS', message: 'Simulation finished successfully.' }];
    
    setTimeout(() => {
      isSimulating = false;
      globalBubble = 'THINKING: Analyzing repo architecture...';
      chief.bubbleText = '';
    }, 3000);
  }

  function init3D() {
    if (!container) return;

    scene = new THREE.Scene();
    scene.background = new THREE.Color(0x0f172a); // Darker professional background

    const aspect = container.clientWidth / container.clientHeight;
    const d = 12; // Slightly wider view
    camera = new THREE.OrthographicCamera(-d * aspect, d * aspect, d, -d, 1, 1000);
    camera.position.set(20, 25, 20);
    camera.lookAt(0, 0, 0);

    // Modern Lighting
    const ambientLight = new THREE.AmbientLight(0xffffff, 0.6);
    scene.add(ambientLight);

    const dirLight = new THREE.DirectionalLight(0xffffff, 0.9);
    dirLight.position.set(10, 30, 20);
    dirLight.castShadow = true;
    dirLight.shadow.camera.left = -15;
    dirLight.shadow.camera.right = 15;
    dirLight.shadow.camera.top = 15;
    dirLight.shadow.camera.bottom = -15;
    scene.add(dirLight);

    const blueLight = new THREE.PointLight(0x3b82f6, 1.5, 20);
    blueLight.position.set(-8, 5, -8);
    scene.add(blueLight);
    
    const purpleLight = new THREE.PointLight(0x8b5cf6, 1.5, 20);
    purpleLight.position.set(8, 5, -8);
    scene.add(purpleLight);

    // Floor - Tech Tiles
    const floorGeo = new THREE.PlaneGeometry(28, 28);
    const floorMat = new THREE.MeshStandardMaterial({ color: 0x1e293b, roughness: 0.2, metalness: 0.8 });
    const floor = new THREE.Mesh(floorGeo, floorMat);
    floor.rotation.x = -Math.PI / 2;
    floor.receiveShadow = true;
    scene.add(floor);

    // Grid Overlay for Sci-Fi feel
    const gridHelper = new THREE.GridHelper(28, 28, 0x334155, 0x1e293b);
    gridHelper.position.y = 0.01;
    scene.add(gridHelper);

    // Walls
    const wallMat = new THREE.MeshStandardMaterial({ color: 0x0f172a, roughness: 0.9 });
    const accentWallMat = new THREE.MeshStandardMaterial({ color: 0x1e293b, roughness: 0.7 });
    
    // Back Wall
    const backWallGeo = new THREE.BoxGeometry(28, 12, 0.5);
    const backWall = new THREE.Mesh(backWallGeo, accentWallMat);
    backWall.position.set(0, 6, -14);
    backWall.receiveShadow = true;
    scene.add(backWall);

    // Left Wall
    const sideWallGeo = new THREE.BoxGeometry(0.5, 12, 28);
    const leftWall = new THREE.Mesh(sideWallGeo, wallMat);
    leftWall.position.set(-14, 6, 0);
    leftWall.receiveShadow = true;
    scene.add(leftWall);

    // Giant Screen on Back Wall
    const screenGeo = new THREE.PlaneGeometry(16, 6);
    const screenMat = new THREE.MeshBasicMaterial({ color: 0x0f172a }); // Dark screen
    const screenBorderGeo = new THREE.BoxGeometry(16.4, 6.4, 0.1);
    const screenBorderMat = new THREE.MeshStandardMaterial({ color: 0x334155 });
    
    const mainScreen = new THREE.Group();
    const sc = new THREE.Mesh(screenGeo, screenMat);
    sc.position.set(0, 0, 0.06);
    const border = new THREE.Mesh(screenBorderGeo, screenBorderMat);
    mainScreen.add(sc, border);
    mainScreen.position.set(0, 6, -13.7);
    scene.add(mainScreen);

    // Rugs under desks
    function createRug(x: number, z: number) {
      const rugGeo = new THREE.PlaneGeometry(6, 6);
      const rugMat = new THREE.MeshStandardMaterial({ color: 0x334155 });
      const rug = new THREE.Mesh(rugGeo, rugMat);
      rug.rotation.x = -Math.PI / 2;
      rug.position.set(x, 0.02, z);
      rug.receiveShadow = true;
      scene.add(rug);
    }
    createRug(-6, -4);
    createRug(0, 2);
    createRug(6, -4);

    // Plants
    function createPlant(x: number, z: number) {
      const potGeo = new THREE.CylinderGeometry(0.5, 0.3, 0.8, 16);
      const potMat = new THREE.MeshStandardMaterial({ color: 0xe2e8f0 });
      const pot = new THREE.Mesh(potGeo, potMat);
      pot.position.set(x, 0.4, z);
      pot.castShadow = true;
      
      const plantGeo = new THREE.DodecahedronGeometry(0.8);
      const plantMat = new THREE.MeshStandardMaterial({ color: 0x10b981, roughness: 0.8 });
      const plant = new THREE.Mesh(plantGeo, plantMat);
      plant.position.set(x, 1.4, z);
      plant.castShadow = true;
      
      const plantGeo2 = new THREE.DodecahedronGeometry(0.6);
      const plant2 = new THREE.Mesh(plantGeo2, plantMat);
      plant2.position.set(x+0.4, 1.8, z);
      plant2.castShadow = true;
      const plant3 = new THREE.Mesh(plantGeo2, plantMat);
      plant3.position.set(x-0.4, 1.6, z+0.4);
      plant3.castShadow = true;

      scene.add(pot, plant, plant2, plant3);
    }
    createPlant(-12, -12);
    createPlant(12, -12);
    createPlant(12, 10);
    createPlant(-12, 10);

    // Server Rack
    const rackGroup = new THREE.Group();
    const rackGeo = new THREE.BoxGeometry(2, 6, 2);
    const rackMat = new THREE.MeshStandardMaterial({ color: 0x111827, metalness: 0.9, roughness: 0.2 });
    const rack = new THREE.Mesh(rackGeo, rackMat);
    rack.position.set(-11, 3, -11);
    rack.castShadow = true;
    rackGroup.add(rack);
    
    for (let i = 0; i < 5; i++) {
      const lightGeo = new THREE.PlaneGeometry(1.6, 0.1);
      const lightMat = new THREE.MeshBasicMaterial({ color: 0x3b82f6 });
      const l = new THREE.Mesh(lightGeo, lightMat);
      l.position.set(-11, 1.5 + i * 0.8, -9.99);
      rackGroup.add(l);
    }
    scene.add(rackGroup);

    // Create IT Desks with Avatars
    createDesk(-6, -4, 'Chief AI', 0x3b82f6);
    createDesk(0, 2, 'Senior Arch', 0x8b5cf6);
    createDesk(6, -4, 'Lead Coder', 0xf59e0b);

    renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setSize(container.clientWidth, container.clientHeight);
    renderer.setPixelRatio(window.devicePixelRatio);
    container.appendChild(renderer.domElement);

    controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.dampingFactor = 0.05;

    animate();
  }

  function createDesk(x: number, z: number, role: string, color: number) {
    const deskGroup = new THREE.Group();
    deskGroup.position.set(x, 0, z);

    // Modern Table top
    const topGeo = new THREE.BoxGeometry(4, 0.1, 2);
    const topMat = new THREE.MeshStandardMaterial({ color: 0x1e293b, metalness: 0.5, roughness: 0.5 }); 
    const top = new THREE.Mesh(topGeo, topMat);
    top.position.y = 1.3;
    top.castShadow = true;
    top.receiveShadow = true;
    deskGroup.add(top);

    // Glowing Desk Edge
    const edgeGeo = new THREE.BoxGeometry(4.05, 0.05, 2.05);
    const edgeMat = new THREE.MeshBasicMaterial({ color: color });
    const edge = new THREE.Mesh(edgeGeo, edgeMat);
    edge.position.y = 1.3;
    deskGroup.add(edge);

    // Desk Legs
    const legGeo = new THREE.CylinderGeometry(0.05, 0.05, 1.3);
    const legMat = new THREE.MeshStandardMaterial({ color: 0x94a3b8, metalness: 0.8 });
    const offsets = [ [-1.8, -0.8], [1.8, -0.8], [-1.8, 0.8], [1.8, 0.8] ];
    offsets.forEach(pos => {
      const leg = new THREE.Mesh(legGeo, legMat);
      leg.position.set(pos[0], 0.65, pos[1]);
      leg.castShadow = true;
      deskGroup.add(leg);
    });

    // Glass Partition
    const partGeo = new THREE.BoxGeometry(4, 1.2, 0.05);
    const partMat = new THREE.MeshStandardMaterial({ color: 0x38bdf8, transparent: true, opacity: 0.3 });
    const partition = new THREE.Mesh(partGeo, partMat);
    partition.position.set(0, 1.9, -1.0);
    deskGroup.add(partition);

    // Dual Monitors (Curved setup)
    const monitorGeo = new THREE.BoxGeometry(1.4, 0.8, 0.05);
    const screenGeo = new THREE.PlaneGeometry(1.3, 0.7);
    const monitorMat = new THREE.MeshStandardMaterial({ color: 0x0f172a }); // Back
    const screenMat = new THREE.MeshBasicMaterial({ color: 0x000000 }); // Screen
    const standGeo = new THREE.CylinderGeometry(0.02, 0.02, 0.3);
    
    // Monitor 1
    const m1 = new THREE.Group();
    const mon1 = new THREE.Mesh(monitorGeo, monitorMat);
    const scr1 = new THREE.Mesh(screenGeo, screenMat);
    scr1.position.z = 0.026;
    m1.add(mon1, scr1);
    m1.position.set(-0.75, 1.8, -0.5);
    m1.rotation.y = Math.PI / 8;
    const stand1 = new THREE.Mesh(standGeo, legMat);
    stand1.position.set(-0.75, 1.5, -0.5);
    
    // Monitor 2
    const m2 = new THREE.Group();
    const mon2 = new THREE.Mesh(monitorGeo, monitorMat);
    const scr2 = new THREE.Mesh(screenGeo, screenMat);
    scr2.position.z = 0.026;
    m2.add(mon2, scr2);
    m2.position.set(0.75, 1.8, -0.5);
    m2.rotation.y = -Math.PI / 8;
    const stand2 = new THREE.Mesh(standGeo, legMat);
    stand2.position.set(0.75, 1.5, -0.5);

    // Keyboard & Mouse
    const kbGeo = new THREE.BoxGeometry(1.0, 0.02, 0.3);
    const kbMat = new THREE.MeshStandardMaterial({ color: 0x334155 });
    const kb = new THREE.Mesh(kbGeo, kbMat);
    kb.position.set(0, 1.36, 0.2);
    const mouseGeo = new THREE.BoxGeometry(0.15, 0.03, 0.2);
    const mouse = new THREE.Mesh(mouseGeo, kbMat);
    mouse.position.set(0.7, 1.36, 0.2);

    deskGroup.add(m1, stand1, m2, stand2, kb, mouse);

    // Office Chair
    const chairGroup = new THREE.Group();
    chairGroup.position.set(0, 0, 1.0);
    
    const seatGeo = new THREE.BoxGeometry(0.8, 0.1, 0.7);
    const chairMat = new THREE.MeshStandardMaterial({ color: 0x111827 });
    const seat = new THREE.Mesh(seatGeo, chairMat);
    seat.position.y = 0.7;
    seat.castShadow = true;
    
    const backGeo = new THREE.BoxGeometry(0.8, 0.9, 0.1);
    const back = new THREE.Mesh(backGeo, chairMat);
    back.position.set(0, 1.2, 0.3);
    back.castShadow = true;
    
    const poleGeo = new THREE.CylinderGeometry(0.05, 0.05, 0.6);
    const pole = new THREE.Mesh(poleGeo, legMat);
    pole.position.y = 0.4;
    
    const baseGeo = new THREE.CylinderGeometry(0.4, 0.4, 0.05, 5);
    const base = new THREE.Mesh(baseGeo, chairMat);
    base.position.y = 0.1;
    
    chairGroup.add(seat, back, pole, base);
    deskGroup.add(chairGroup);

    scene.add(deskGroup);

    // Place Agent slightly outside the desk
    createAgentAvatar(role, color, x, z + 1.2);
  }

  function createAgentAvatar(name: string, color: number, x: number, z: number) {
    const group = new THREE.Group();
    group.position.set(x, 0, z);

    // Armor color (Desert Tan / Beige like the image)
    const armorColor = 0xd2b48c; // Tan
    const darkSuitColor = 0x2f3640; // Dark grey undersuit

    // Materials
    const armorMat = new THREE.MeshPhongMaterial({ color: armorColor, shininess: 30 });
    const suitMat = new THREE.MeshPhongMaterial({ color: darkSuitColor, shininess: 10 });
    const visorMat = new THREE.MeshPhongMaterial({ color: 0x111111, shininess: 100 }); // Dark shiny visor

    // Main Torso (Armor)
    const torsoGeo = new THREE.BoxGeometry(0.6, 0.7, 0.4);
    const torso = new THREE.Mesh(torsoGeo, armorMat);
    torso.position.y = 0.9;
    
    // Chest plate with Agent specific color
    const chestGeo = new THREE.BoxGeometry(0.62, 0.3, 0.42);
    const chestMat = new THREE.MeshPhongMaterial({ color: color, shininess: 50 });
    const chest = new THREE.Mesh(chestGeo, chestMat);
    chest.position.y = 1.0;
    group.add(torso, chest);

    // Head / Helmet
    const headGroup = new THREE.Group();
    headGroup.position.y = 1.45;
    
    const helmetGeo = new THREE.BoxGeometry(0.4, 0.45, 0.4);
    const helmet = new THREE.Mesh(helmetGeo, armorMat);
    
    const visorGeo = new THREE.BoxGeometry(0.32, 0.25, 0.42);
    const visor = new THREE.Mesh(visorGeo, visorMat);
    visor.position.set(0, 0.05, 0.02);
    
    headGroup.add(helmet, visor);
    group.add(headGroup);

    // Legs
    const legGeo = new THREE.BoxGeometry(0.25, 0.6, 0.25);
    
    // Left Leg
    const leftLegGroup = new THREE.Group();
    leftLegGroup.position.set(-0.2, 0.6, 0); // Pivot point at hip
    const leftLeg = new THREE.Mesh(legGeo, armorMat);
    leftLeg.position.y = -0.3; // Offset so it hangs down from pivot
    leftLegGroup.add(leftLeg);
    group.add(leftLegGroup);

    // Right Leg
    const rightLegGroup = new THREE.Group();
    rightLegGroup.position.set(0.2, 0.6, 0);
    const rightLeg = new THREE.Mesh(legGeo, armorMat);
    rightLeg.position.y = -0.3;
    rightLegGroup.add(rightLeg);
    group.add(rightLegGroup);

    // Arms
    const armGeo = new THREE.BoxGeometry(0.2, 0.6, 0.2);

    // Left Arm
    const leftArmGroup = new THREE.Group();
    leftArmGroup.position.set(-0.45, 1.2, 0); // Pivot at shoulder
    const leftArm = new THREE.Mesh(armGeo, armorMat);
    leftArm.position.y = -0.3;
    leftArmGroup.add(leftArm);
    group.add(leftArmGroup);

    // Right Arm
    const rightArmGroup = new THREE.Group();
    rightArmGroup.position.set(0.45, 1.2, 0);
    const rightArm = new THREE.Mesh(armGeo, armorMat);
    rightArm.position.y = -0.3;
    rightArmGroup.add(rightArm);
    group.add(rightArmGroup);

    scene.add(group);

    agents3D.push({ 
      mesh: group, 
      name, 
      targetX: x, 
      targetZ: z, 
      bubbleText: '',
      parts: { leftLeg: leftLegGroup, rightLeg: rightLegGroup, leftArm: leftArmGroup, rightArm: rightArmGroup }
    });
  }

  function animate() {
    animId = requestAnimationFrame(animate);

    // Idle animation bobbing and movement interpolation
    const time = Date.now() * 0.003;
    agents3D.forEach((a, idx) => {
      // Smooth movement towards target
      const dx = a.targetX - a.mesh.position.x;
      const dz = a.targetZ - a.mesh.position.z;
      
      const isMoving = Math.abs(dx) > 0.01 || Math.abs(dz) > 0.01;

      if (isMoving) {
        a.mesh.position.x += dx * 0.05;
        a.mesh.position.z += dz * 0.05;

        // Face movement direction
        const targetAngle = Math.atan2(dx, dz);
        let diff = targetAngle - a.mesh.rotation.y;
        while (diff < -Math.PI) diff += Math.PI * 2;
        while (diff > Math.PI) diff -= Math.PI * 2;
        a.mesh.rotation.y += diff * 0.1;

        // Stand up height
        a.mesh.position.y = Math.abs(Math.sin(time * 12 * 2)) * 0.05;

        // Walk cycle animation (arms and legs)
        const walkSpeed = 12;
        const walkAngle = Math.sin(time * walkSpeed) * 0.8;
        if (a.parts) {
          a.parts.leftLeg.rotation.x = walkAngle;
          a.parts.rightLeg.rotation.x = -walkAngle;
          a.parts.leftArm.rotation.x = -walkAngle;
          a.parts.rightArm.rotation.x = walkAngle;
        }
      } else {
        // Sitting Pose on Chair
        a.mesh.rotation.y *= 0.85; // Rotate back to face monitors (0 rad)
        a.mesh.position.y = -0.25; // Lower body onto chair seat

        if (a.parts) {
          // Bend legs 90 deg forward for sitting
          a.parts.leftLeg.rotation.x = -Math.PI / 2;
          a.parts.rightLeg.rotation.x = -Math.PI / 2;
          // Rest arms on desk
          a.parts.leftArm.rotation.x = -Math.PI / 4;
          a.parts.rightArm.rotation.x = -Math.PI / 4;
        }
      }

      if (!uiAgents[idx]) {
        uiAgents[idx] = { id: idx, name: a.name, text: '', x: 0, y: 0, visible: false };
      }

      if (a.bubbleText) {
        const vector = new THREE.Vector3();
        vector.setFromMatrixPosition(a.mesh.matrixWorld);
        vector.y += 2.2; // Above head
        vector.project(camera);

        uiAgents[idx].x = (vector.x * .5 + .5) * container.clientWidth;
        uiAgents[idx].y = (vector.y * -.5 + .5) * container.clientHeight;
        uiAgents[idx].text = a.bubbleText;
        uiAgents[idx].visible = true;
      } else {
        uiAgents[idx].visible = false;
      }
    });

    uiAgents = uiAgents; // trigger svelte reactivity

    if (controls) controls.update();

    renderer.render(scene, camera);
  }

  function handleResize() {
    if (!container || !renderer || !camera) return;
    const aspect = container.clientWidth / container.clientHeight;
    const d = 10;
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
  <div class="absolute top-6 left-1/2 -translate-x-1/2 bg-surface-container-lowest border border-outline-variant px-4 py-2 rounded-xl text-xs font-semibold shadow-md animate-bounce">
    <span class="text-primary font-bold">{isSimulating ? 'SIMULATING:' : 'IDLE:'}</span> {globalBubble}
  </div>

  <!-- Agent Speech Bubbles -->
  {#each uiAgents as uiAgent}
    {#if uiAgent.visible}
      <div 
        class="absolute -translate-x-1/2 -translate-y-full bg-white border border-outline-variant px-3 py-1.5 rounded-xl text-[10px] font-semibold shadow-lg whitespace-nowrap pointer-events-none transition-all duration-75"
        style="left: {uiAgent.x}px; top: {uiAgent.y}px;"
      >
        <div class="absolute -bottom-1.5 left-1/2 -translate-x-1/2 w-3 h-3 bg-white border-b border-r border-outline-variant rotate-45"></div>
        <span class="text-primary">{uiAgent.name}:</span> {uiAgent.text}
      </div>
    {/if}
  {/each}

  <!-- Control Panel -->
  <div class="absolute top-6 right-6 bg-surface-container-lowest border border-outline-variant px-4 py-3 rounded-xl shadow-md space-y-3 w-64">
    <h3 class="text-xs font-bold text-on-surface uppercase tracking-wider">Simulation Controls</h3>
    <p class="text-[10px] text-on-surface-variant leading-tight">Run a test to simulate agent collaboration and log activities.</p>
    <button on:click={handleTestRun} disabled={isSimulating}
      class="w-full bg-primary text-on-primary px-3 py-2 rounded-lg text-xs font-bold hover:brightness-110 flex items-center justify-center gap-2 transition-all disabled:opacity-50">
      <span class="material-symbols-outlined text-sm">{isSimulating ? 'sync' : 'play_arrow'}</span> {isSimulating ? 'RUNNING...' : 'TEST RUN'}
    </button>
  </div>

  <!-- Bottom Real-time Console Log Overlay -->
  <div class="absolute bottom-6 left-6 w-96 bg-surface-container-lowest border border-outline-variant rounded-xl p-3 shadow-xl space-y-2">
    <div class="flex items-center justify-between border-b border-outline-variant pb-1">
      <span class="text-[10px] font-bold uppercase text-on-surface-variant flex items-center gap-1">
        <span class="material-symbols-outlined text-xs">terminal</span> Console Logs
      </span>
      <span class="text-[9px] font-bold uppercase text-primary bg-primary-container px-2 py-0.5 rounded">REAL-TIME 3D</span>
    </div>
    <div class="font-mono text-[11px] space-y-1 max-h-28 overflow-y-auto">
      {#each $logs.slice(-4) as log}
        <div class="flex gap-2">
          <span class="text-secondary font-bold">[{log.level}]</span>
          <span class="text-on-surface line-clamp-1">{log.message}</span>
        </div>
      {:else}
        <div class="text-on-surface-variant italic">3D Office Active. Synchronizing team heartbeat...</div>
      {/each}
    </div>
  </div>
</div>
