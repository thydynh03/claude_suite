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

    // Floor
    const floorGeo = new THREE.PlaneGeometry(16, 16);
    const floorMat = new THREE.MeshStandardMaterial({ color: 0xeeceef0, side: THREE.DoubleSide });
    const floor = new THREE.Mesh(floorGeo, floorMat);
    floor.rotation.x = -Math.PI / 2;
    scene.add(floor);

    // Grid Helper
    const gridHelper = new THREE.GridHelper(16, 16, 0xc3c6d7, 0xe0e3e5);
    gridHelper.position.y = 0.01;
    scene.add(gridHelper);

    // Create Desks
    createDesk(-4, -2);
    createDesk(0, 2);
    createDesk(4, -2);

    // Create Agent Avatars
    createAgentAvatar('Chief', 0x004ac6, -4, -2);
    createAgentAvatar('Senior Architect', 0x515f74, 0, 2);
    createAgentAvatar('Lead Coder', 0x943700, 4, -2);

    renderer = new THREE.WebGLRenderer({ antialias: true });
    renderer.setSize(container.clientWidth, container.clientHeight);
    renderer.setPixelRatio(window.devicePixelRatio);
    container.appendChild(renderer.domElement);

    animate();
  }

  function createDesk(x: number, z: number) {
    const deskGroup = new THREE.Group();
    deskGroup.position.set(x, 0, z);

    // Table top
    const topGeo = new THREE.BoxGeometry(2.5, 0.2, 1.5);
    const topMat = new THREE.MeshStandardMaterial({ color: 0xffffff });
    const top = new THREE.Mesh(topGeo, topMat);
    top.position.y = 1.2;
    deskGroup.add(top);

    // Laptop
    const laptopGeo = new THREE.BoxGeometry(0.6, 0.05, 0.4);
    const laptopMat = new THREE.MeshStandardMaterial({ color: 0x2563eb });
    const laptop = new THREE.Mesh(laptopGeo, laptopMat);
    laptop.position.set(0, 1.3, 0);
    deskGroup.add(laptop);

    scene.add(deskGroup);
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

    // Head
    const headGeo = new THREE.SphereGeometry(0.35, 16, 16);
    const headMat = new THREE.MeshStandardMaterial({ color: 0xffdbcd });
    const head = new THREE.Mesh(headGeo, headMat);
    head.position.y = 1.5;
    group.add(head);

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
