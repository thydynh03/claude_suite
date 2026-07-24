<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as THREE from 'three';
  import { logs } from '../../lib/stores/appState';

  let container: HTMLDivElement;
  let scene: THREE.Scene;
  let camera: THREE.OrthographicCamera;
  let renderer: THREE.WebGLRenderer;
  let animId: number;

  let agents3D: { mesh: THREE.Group; name: string; targetX: number; targetZ: number }[] = [];

  onMount(() => {
    init3D();
    window.addEventListener('resize', handleResize);
  });

  onDestroy(() => {
    if (animId) cancelAnimationFrame(animId);
    window.removeEventListener('resize', handleResize);
  });

  function init3D() {
    if (!container) return;

    scene = new THREE.Scene();
    scene.background = new THREE.Color(0xf7f9fb);

    const aspect = container.clientWidth / container.clientHeight;
    const d = 10;
    camera = new THREE.OrthographicCamera(-d * aspect, d * aspect, d, -d, 1, 1000);
    camera.position.set(20, 20, 20);
    camera.lookAt(0, 0, 0);

    // Lights
    const ambientLight = new THREE.AmbientLight(0xffffff, 0.7);
    scene.add(ambientLight);

    const dirLight = new THREE.DirectionalLight(0xffffff, 0.8);
    dirLight.position.set(15, 25, 15);
    scene.add(dirLight);

    // Floor (Darker for modern IT feel)
    const floorGeo = new THREE.PlaneGeometry(24, 24);
    const floorMat = new THREE.MeshStandardMaterial({ color: 0x1f2937, side: THREE.DoubleSide });
    const floor = new THREE.Mesh(floorGeo, floorMat);
    floor.rotation.x = -Math.PI / 2;
    scene.add(floor);

    // Walls
    const wallMat = new THREE.MeshStandardMaterial({ color: 0x374151 }); // Dark Slate
    const glassMat = new THREE.MeshStandardMaterial({ color: 0x7dd3fc, transparent: true, opacity: 0.25 });
    
    // Back Wall
    const backWallGeo = new THREE.BoxGeometry(24, 8, 0.5);
    const backWall = new THREE.Mesh(backWallGeo, wallMat);
    backWall.position.set(0, 4, -12);
    scene.add(backWall);

    // Glass Wall (Front)
    const glassWallGeo = new THREE.BoxGeometry(24, 8, 0.2);
    const glassWall = new THREE.Mesh(glassWallGeo, glassMat);
    glassWall.position.set(0, 4, 12);
    scene.add(glassWall);

    // Whiteboard on back wall
    const wbGeo = new THREE.BoxGeometry(10, 4, 0.2);
    const wbMat = new THREE.MeshStandardMaterial({ color: 0xf8fafc });
    const wb = new THREE.Mesh(wbGeo, wbMat);
    wb.position.set(0, 4.5, -11.7);
    scene.add(wb);

    // Rug
    const rugGeo = new THREE.PlaneGeometry(16, 12);
    const rugMat = new THREE.MeshStandardMaterial({ color: 0x475569 });
    const rug = new THREE.Mesh(rugGeo, rugMat);
    rug.rotation.x = -Math.PI / 2;
    rug.position.y = 0.02;
    scene.add(rug);

    // Grid Helper
    const gridHelper = new THREE.GridHelper(24, 24, 0x475569, 0x334155);
    gridHelper.position.y = 0.05;
    scene.add(gridHelper);

    // Plant 1
    const potGeo = new THREE.CylinderGeometry(0.5, 0.3, 0.8, 16);
    const potMat = new THREE.MeshStandardMaterial({ color: 0xd4d4d8 });
    const pot = new THREE.Mesh(potGeo, potMat);
    pot.position.set(-10, 0.4, -10);
    const plantGeo = new THREE.SphereGeometry(0.8, 16, 16);
    const plantMat = new THREE.MeshStandardMaterial({ color: 0x22c55e });
    const plant = new THREE.Mesh(plantGeo, plantMat);
    plant.position.set(-10, 1.4, -10);
    scene.add(pot, plant);

    // Create IT Desks with Avatars
    createDesk(-6, -4, 'Chief AI', 0x3b82f6);
    createDesk(0, 2, 'Senior Arch', 0x8b5cf6);
    createDesk(6, -4, 'Lead Coder', 0xf59e0b);

    renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setSize(container.clientWidth, container.clientHeight);
    renderer.setPixelRatio(window.devicePixelRatio);
    container.appendChild(renderer.domElement);

    animate();
  }

  function createDesk(x: number, z: number, role: string, color: number) {
    const deskGroup = new THREE.Group();
    deskGroup.position.set(x, 0, z);

    // Table top
    const topGeo = new THREE.BoxGeometry(3.5, 0.1, 1.6);
    const topMat = new THREE.MeshStandardMaterial({ color: 0x0f172a }); // Dark blue wood
    const top = new THREE.Mesh(topGeo, topMat);
    top.position.y = 1.3;
    deskGroup.add(top);

    // Desk Legs
    const legGeo = new THREE.CylinderGeometry(0.05, 0.05, 1.3);
    const legMat = new THREE.MeshStandardMaterial({ color: 0x94a3b8 });
    const offsets = [
      [-1.6, -0.6], [1.6, -0.6], [-1.6, 0.6], [1.6, 0.6]
    ];
    offsets.forEach(pos => {
      const leg = new THREE.Mesh(legGeo, legMat);
      leg.position.set(pos[0], 0.65, pos[1]);
      deskGroup.add(leg);
    });

    // Dual Monitors
    const monitorGeo = new THREE.BoxGeometry(1.2, 0.8, 0.05);
    const monitorMat = new THREE.MeshStandardMaterial({ color: 0x000000 }); // Screen
    const standGeo = new THREE.CylinderGeometry(0.02, 0.02, 0.3);
    
    // Monitor 1 (Left)
    const monitor1 = new THREE.Mesh(monitorGeo, monitorMat);
    monitor1.position.set(-0.65, 1.8, -0.4);
    monitor1.rotation.y = Math.PI / 8;
    const stand1 = new THREE.Mesh(standGeo, legMat);
    stand1.position.set(-0.65, 1.5, -0.4);
    
    // Monitor 2 (Right)
    const monitor2 = new THREE.Mesh(monitorGeo, monitorMat);
    monitor2.position.set(0.65, 1.8, -0.4);
    monitor2.rotation.y = -Math.PI / 8;
    const stand2 = new THREE.Mesh(standGeo, legMat);
    stand2.position.set(0.65, 1.5, -0.4);

    deskGroup.add(monitor1, stand1, monitor2, stand2);
    scene.add(deskGroup);

    // Place Agent
    createAgentAvatar(role, color, x, z + 0.7);
  }

  function createAgentAvatar(name: string, color: number, x: number, z: number) {
    const group = new THREE.Group();
    group.position.set(x, 0, z);

    // Body
    const bodyGeo = new THREE.CylinderGeometry(0.4, 0.4, 1.2, 16);
    const bodyMat = new THREE.MeshStandardMaterial({ color: color });
    const body = new THREE.Mesh(bodyGeo, bodyMat);
    body.position.y = 0.6;
    group.add(body);

    // Head (Monitor/Robot style)
    const headGeo = new THREE.BoxGeometry(0.7, 0.6, 0.6);
    const headMat = new THREE.MeshStandardMaterial({ color: 0xe2e8f0 });
    const head = new THREE.Mesh(headGeo, headMat);
    head.position.y = 1.5;
    
    // Visor (Screen)
    const visorGeo = new THREE.PlaneGeometry(0.5, 0.2);
    const visorMat = new THREE.MeshStandardMaterial({ color: 0x10b981 });
    const visor = new THREE.Mesh(visorGeo, visorMat);
    visor.position.set(0, 1.5, 0.31);
    
    group.add(head, visor);

    scene.add(group);

    agents3D.push({ mesh: group, name, targetX: x, targetZ: z });
  }

  function animate() {
    animId = requestAnimationFrame(animate);

    // Idle animation bobbing
    const time = Date.now() * 0.003;
    agents3D.forEach((a, idx) => {
      a.mesh.position.y = Math.sin(time + idx) * 0.05;
    });

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

  <!-- Speech Bubble Overlay -->
  <div class="absolute top-12 left-1/2 -translate-x-1/2 bg-surface-container-lowest border border-outline-variant px-4 py-2 rounded-xl text-xs font-semibold shadow-md animate-bounce">
    <span class="text-primary font-bold">THINKING:</span> Analyzing repo architecture...
  </div>

  <!-- Control Panel -->
  <div class="absolute top-6 right-6 bg-surface-container-lowest border border-outline-variant px-4 py-3 rounded-xl shadow-md space-y-3 w-64">
    <h3 class="text-xs font-bold text-on-surface uppercase tracking-wider">Simulation Controls</h3>
    <p class="text-[10px] text-on-surface-variant leading-tight">Run a test to simulate agent collaboration and log activities.</p>
    <button on:click={() => { 
        alert('Test Run Initiated! Simulating 3D behaviors...'); 
        $logs = [...$logs, { time: new Date().toLocaleTimeString(), level: 'INFO', message: 'Initiated 3D Simulation Test Run' }];
      }} 
      class="w-full bg-primary text-on-primary px-3 py-2 rounded-lg text-xs font-bold hover:brightness-110 flex items-center justify-center gap-2 transition-all">
      <span class="material-symbols-outlined text-sm">play_arrow</span> TEST RUN
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
