// The traveller of the navigation in 3D: a car, a cyclist or a walker, drawn with three.js in a
// custom layer of MapLibre. It keeps its size on the screen at every zoom, stands on the terrain
// when the map is 3D, glides to each new position in the time the camera takes, and its wheels,
// pedals or legs move with its speed. app.js loads this file only for navigation.
import * as THREE from "./vendor/three-0.170.0/three.module.min.js";

const BLUE = 0x1a5fd0;
const UP = new THREE.Vector3(0, 1, 0);
const mat = (color, o = {}) => new THREE.MeshStandardMaterial({ color, roughness: 0.6, metalness: 0.05, ...o });

// The models are in pixels of the screen, y up, the front towards -z.

// a cylinder from a to b, which place() moves again
function rod(a, b, r, m, seg = 10) {
  const mesh = new THREE.Mesh(new THREE.CylinderGeometry(r, r, 1, seg), m);
  place(mesh, a, b);
  return mesh;
}
const tmp = new THREE.Vector3();
function place(mesh, a, b) {
  tmp.subVectors(b, a);
  const len = tmp.length() || 1e-6;
  mesh.scale.set(1, len, 1);
  mesh.position.copy(a).addScaledVector(tmp, 0.5);
  mesh.quaternion.setFromUnitVectors(UP, tmp.divideScalar(len));
}
const v = (x, y, z) => new THREE.Vector3(x, y, z);

// a side profile (forward, up) extruded across the width
function side(points, width, m, bevel = 0.8) {
  const s = new THREE.Shape();
  points.forEach(([f, u], i) => (i ? s.lineTo(f, u) : s.moveTo(f, u)));
  s.closePath();
  const g = new THREE.ExtrudeGeometry(s, { depth: width - 2 * bevel, bevelEnabled: bevel > 0, bevelThickness: bevel, bevelSize: bevel, bevelSegments: 2, curveSegments: 4 });
  g.translate(0, 0, -(width - 2 * bevel) / 2);
  g.rotateY(Math.PI / 2); // forward → -z, the width → x
  return new THREE.Mesh(g, m);
}

// a soft round shadow on the ground
function shadow(w, l) {
  const c = document.createElement("canvas");
  c.width = c.height = 64;
  const x = c.getContext("2d"), g = x.createRadialGradient(32, 32, 4, 32, 32, 32);
  g.addColorStop(0, "rgba(0,0,0,.42)");
  g.addColorStop(1, "rgba(0,0,0,0)");
  x.fillStyle = g;
  x.fillRect(0, 0, 64, 64);
  const m = new THREE.Mesh(new THREE.PlaneGeometry(w, l), new THREE.MeshBasicMaterial({ map: new THREE.CanvasTexture(c), transparent: true, depthWrite: false }));
  m.rotation.x = -Math.PI / 2;
  m.position.y = 0.2;
  return m;
}

function wheel(r, w, tyre, rim) {
  const g = new THREE.Group();
  const t = new THREE.Mesh(new THREE.CylinderGeometry(r, r, w, 20), tyre);
  t.rotation.z = Math.PI / 2;
  const hub = new THREE.Mesh(new THREE.CylinderGeometry(r * 0.55, r * 0.55, w + 0.3, 12), rim);
  hub.rotation.z = Math.PI / 2;
  const spoke = new THREE.Mesh(new THREE.BoxGeometry(w + 0.5, r * 1.05, r * 0.22), tyre); // shows the turning
  g.add(t, hub, spoke);
  return g;
}

function car() {
  const g = new THREE.Group();
  const paint = mat(BLUE, { roughness: 0.32, metalness: 0.35 });
  const glass = mat(0x1b2633, { roughness: 0.15, metalness: 0.6 });
  const tyre = mat(0x16181b, { roughness: 0.9 });
  const rim = mat(0xc9ced6, { metalness: 0.7, roughness: 0.3 });
  g.add(shadow(30, 54));
  g.add(side([[-22, 3], [21, 3], [23, 5.5], [22.5, 8.6], [11, 10.2], [-17, 10.6], [-21.8, 9.4], [-22.6, 6]], 21, paint));
  g.add(side([[10.5, 10.2], [3.5, 16.6], [-11.5, 16.6], [-16.5, 10.6]], 18, glass, 0.6));
  const roof = new THREE.Mesh(new THREE.BoxGeometry(17.2, 1.1, 14.5), paint);
  roof.position.set(0, 17.1, 3.8);
  g.add(roof);
  const lamp = (x, z, color) => {
    const m = new THREE.Mesh(new THREE.BoxGeometry(5, 1.8, 0.8), mat(color, { emissive: color, emissiveIntensity: 0.9 }));
    m.position.set(x, 7.4, z);
    return m;
  };
  g.add(lamp(-6.6, -23.6, 0xfff1c2), lamp(6.6, -23.6, 0xfff1c2), lamp(-6.8, 23, 0xd8262f), lamp(6.8, 23, 0xd8262f));
  const wheels = [];
  for (const [x, z] of [[-9.6, -13.5], [9.6, -13.5], [-9.6, 14], [9.6, 14]]) {
    const w = wheel(4.3, 3.2, tyre, rim);
    w.position.set(x, 4.3, z);
    wheels.push(w);
    g.add(w);
  }
  let turn = 0;
  return {
    group: g,
    move(dist, dt) { // dist in metres; the wheels as of a car of 4.5 m
      turn -= Math.min(dist / 0.33, dt * 14);
      for (const w of wheels) w.rotation.x = turn;
    },
  };
}

function bike() {
  const g = new THREE.Group();
  const frame = mat(BLUE, { roughness: 0.35, metalness: 0.4 });
  const black = mat(0x1d2025, { roughness: 0.8 });
  const steel = mat(0xb9c0c8, { metalness: 0.8, roughness: 0.3 });
  const jersey = mat(0xf08c00), shorts = mat(0x22262c), skin = mat(0xd9a77c), helmet = mat(0xf5f7fa, { roughness: 0.3 });
  g.add(shadow(16, 42));
  const R = 7;
  const wheels = [];
  for (const z of [11, -11]) {
    const w = new THREE.Group();
    const tyre = new THREE.Mesh(new THREE.TorusGeometry(R, 0.9, 8, 32), black);
    tyre.rotation.y = Math.PI / 2;
    const s1 = new THREE.Mesh(new THREE.BoxGeometry(0.25, 2 * R, 0.25), steel), s2 = s1.clone();
    s2.rotation.x = Math.PI / 2;
    const s3 = s1.clone(), s4 = s1.clone();
    s3.rotation.x = Math.PI / 4;
    s4.rotation.x = -Math.PI / 4;
    w.add(tyre, s1, s2, s3, s4);
    w.position.set(0, R, z);
    wheels.push(w);
    g.add(w);
  }
  const rear = v(0, R, 11), front = v(0, R, -11), bb = v(0, 6.2, 1), seat = v(0, 17, 4.2), head = v(0, 17.6, -7), headLow = v(0, 13.5, -8.3);
  const bar = v(0, 19.4, -6.2);
  for (const [a, b] of [[rear, bb], [rear, seat], [bb, seat], [v(0, 16, 3.8), head], [bb, headLow], [headLow, front], [head, headLow]]) g.add(rod(a, b, 0.75, frame));
  g.add(rod(head, bar, 0.6, steel), rod(v(-4.2, 19.4, -6.2), v(4.2, 19.4, -6.2), 0.55, black));
  const saddle = new THREE.Mesh(new THREE.BoxGeometry(2.6, 0.9, 5), black);
  saddle.position.set(0, 18, 4.6);
  g.add(saddle);
  // the rider, leaning forward
  const hip = v(0, 19.4, 4.6), neck = v(0, 27.2, -1.6);
  g.add(rod(hip, neck, 3, jersey, 12));
  const back = new THREE.Mesh(new THREE.SphereGeometry(3, 12, 8), jersey);
  back.position.copy(neck);
  g.add(back);
  const hd = new THREE.Mesh(new THREE.SphereGeometry(2.7, 16, 12), skin);
  hd.position.set(0, 30.6, -3.6);
  const hl = new THREE.Mesh(new THREE.SphereGeometry(3.05, 16, 8, 0, Math.PI * 2, 0, Math.PI / 2), helmet);
  hl.position.set(0, 30.9, -3.4);
  hl.rotation.x = -0.35;
  g.add(hd, hl);
  for (const x of [-3, 3]) g.add(rod(v(x * 1.1, 26.8, -1.6), v(x * 1.35, 19.6, -6), 0.95, skin));
  // legs on the pedals: thigh and shin found again each frame
  const legs = [-1, 1].map((sx) => {
    const thigh = rod(hip, hip, 1.45, shorts), shin = rod(hip, hip, 1.1, skin), crank = rod(bb, bb, 0.45, steel);
    const shoe = new THREE.Mesh(new THREE.BoxGeometry(1.8, 1, 3.4), black);
    g.add(thigh, shin, crank, shoe);
    return { sx, thigh, shin, crank, shoe };
  });
  const L1 = 9, L2 = 8.8, crankR = 3.4;
  let a = 0, turn = 0;
  const pose = () => {
    for (const l of legs) {
      const t = a + (l.sx > 0 ? 0 : Math.PI);
      const pedal = v(l.sx * 2.6, bb.y + crankR * Math.cos(t), bb.z + crankR * Math.sin(t));
      const h = v(l.sx * 2.2, hip.y, hip.z);
      // two bones from the hip to the pedal, the knee forward
      const dy = pedal.y - h.y, dz = pedal.z - h.z, d = Math.min(Math.hypot(dy, dz), L1 + L2 - 0.01);
      const uy = dy / d, uz = dz / d, along = (L1 * L1 - L2 * L2 + d * d) / (2 * d), up = Math.sqrt(Math.max(0, L1 * L1 - along * along));
      let py = -uz, pz = uy;
      if (pz > 0) { py = -py; pz = -pz; }
      const knee = v(l.sx * 2.4, h.y + uy * along + py * up, h.z + uz * along + pz * up);
      place(l.thigh, h, knee);
      place(l.shin, knee, pedal);
      place(l.crank, v(l.sx * 1.6, bb.y, bb.z), pedal);
      l.shoe.position.copy(pedal).add(v(0, 0.3, -0.6));
    }
  };
  pose();
  return {
    group: g,
    move(dist, dt) {
      turn -= Math.min(dist / 0.34, dt * 16);
      for (const w of wheels) w.rotation.x = turn;
      a -= Math.min(dist / 1.0, dt * 9); // one turn of the pedals in about 6 m
      pose();
    },
  };
}

function walker() {
  const g = new THREE.Group();
  const shirt = mat(BLUE), trousers = mat(0x2b3138), skin = mat(0xd9a77c), shoes = mat(0x1d2025), pack = mat(0x2f6f4f), hair = mat(0x3b2a20);
  g.add(shadow(16, 16));
  const limb = (x, y, len, r, m, foot) => {
    const p = new THREE.Group();
    p.position.set(x, y, 0);
    const l = new THREE.Mesh(new THREE.CapsuleGeometry(r, len - 2 * r, 4, 10), m);
    l.position.y = -len / 2;
    p.add(l);
    if (foot) {
      const f = new THREE.Mesh(new THREE.BoxGeometry(2.6, 1.6, 4.4), foot);
      f.position.set(0, -len + 0.4, -1);
      p.add(f);
    }
    g.add(p);
    return p;
  };
  const legs = [limb(-2.1, 14.5, 13.6, 1.75, trousers, shoes), limb(2.1, 14.5, 13.6, 1.75, trousers, shoes)];
  const body = new THREE.Mesh(new THREE.CapsuleGeometry(4, 6.5, 4, 14), shirt);
  body.position.y = 20.2;
  body.scale.z = 0.72;
  const bag = new THREE.Mesh(new THREE.BoxGeometry(6, 7.5, 3), pack);
  bag.position.set(0, 21, 3.9);
  const neck = new THREE.Mesh(new THREE.CylinderGeometry(1.2, 1.2, 2, 8), skin);
  neck.position.y = 26.4;
  const head = new THREE.Mesh(new THREE.SphereGeometry(3.3, 18, 14), skin);
  head.position.y = 29.6;
  const top = new THREE.Mesh(new THREE.SphereGeometry(3.45, 18, 8, 0, Math.PI * 2, 0, Math.PI / 2.1), hair);
  top.position.set(0, 29.8, 0.25);
  g.add(body, bag, neck, head, top);
  const arms = [limb(-5, 24.5, 11, 1.25, shirt), limb(5, 24.5, 11, 1.25, shirt)];
  arms[0].rotation.z = -0.08;
  arms[1].rotation.z = 0.08;
  let a = 0;
  return {
    group: g,
    move(dist, dt) {
      a += Math.min(dist / 0.42, dt * 11); // a step of 0.75 m is half a swing
      const s = Math.sin(a) * Math.min(1, dist / (dt * 0.6 + 1e-6));
      legs[0].rotation.x = s * 0.55;
      legs[1].rotation.x = -s * 0.55;
      arms[0].rotation.x = -s * 0.6;
      arms[1].rotation.x = s * 0.6;
      body.position.y = 20.2 + Math.abs(Math.cos(a)) * 0.5;
    },
  };
}

const MODELS = { car, bike, walk: walker };

// avatar adds the traveller of mode to the map; set() moves it, remove() takes it away
export function avatar(map, maplibregl, mode) {
  const model = MODELS[mode]();
  const scene = new THREE.Scene();
  const camera = new THREE.Camera();
  const turn = new THREE.Group(); // the heading, about the up of the model
  turn.add(model.group);
  const upright = new THREE.Group(); // y up → z up of the map
  upright.rotation.x = Math.PI / 2;
  upright.add(turn);
  scene.add(upright);
  scene.add(new THREE.AmbientLight(0xffffff, 1.1));
  const sky = new THREE.HemisphereLight(0xdfeaff, 0x5a5146, 1.2);
  sky.position.set(0, 0, 1);
  scene.add(sky);
  const sun = new THREE.DirectionalLight(0xffffff, 2.4);
  sun.position.set(-40, -30, 90); // high in the south-west
  scene.add(sun);

  const size = mode === "car" ? 2 : 1.9; // pixels of the screen per unit of the model
  let renderer = null;
  let from = null, to = null, t0 = 0, dur = 0, hFrom = 0, hTo = 0, speed = 0, last = 0;
  const now = () => performance.now();
  const state = () => {
    const k = dur > 0 ? Math.min(1, (now() - t0) / dur) : 1;
    const ll = [from[0] + (to[0] - from[0]) * k, from[1] + (to[1] - from[1]) * k];
    const dh = ((hTo - hFrom + 540) % 360) - 180;
    return { ll, heading: hFrom + dh * k, k };
  };
  const layer = {
    id: "lm-avatar",
    type: "custom",
    renderingMode: "3d",
    onAdd(m, gl) {
      renderer ||= new THREE.WebGLRenderer({ canvas: m.getCanvas(), context: gl, antialias: true });
      renderer.autoClear = false;
    },
    render(gl, args) {
      if (!to) return;
      const t = now(), dt = last ? Math.min(0.1, (t - last) / 1000) : 0;
      last = t;
      const { ll, heading, k } = state();
      model.move(speed * dt, dt);
      turn.rotation.y = (-heading * Math.PI) / 180;
      const alt = map.queryTerrainElevation?.(ll) || 0;
      const mc = maplibregl.MercatorCoordinate.fromLngLat(ll, alt);
      // from straight above a traveller shows less of itself: larger on a flat map
      const s = (size * (1 + 0.5 * Math.max(0, 1 - map.getPitch() / 60))) / (512 * 2 ** map.getZoom());
      const l = new THREE.Matrix4().makeTranslation(mc.x, mc.y, mc.z).scale(new THREE.Vector3(s, -s, s));
      camera.projectionMatrix = new THREE.Matrix4().fromArray(args.defaultProjectionData.mainMatrix).multiply(l);
      renderer.resetState();
      renderer.clearDepth(); // over the buildings and the hills, not in them
      renderer.render(scene, camera);
      if (k < 1 || speed > 0.2) map.triggerRepaint();
      else last = 0;
    },
  };
  map.addLayer(layer);
  const again = () => { if (!map.getLayer(layer.id)) try { map.addLayer(layer); } catch {} };
  map.on("styledata", again); // a new map style drops the layer
  return {
    // to ll (lon, lat) facing heading (degrees from north), in ms, at speed m/s
    set(ll, h, ms = 0, sp = 0) {
      if (!to) { from = to = ll; hFrom = hTo = h; }
      else {
        const s = state();
        from = s.ll;
        hFrom = s.heading;
        to = ll;
        hTo = h;
      }
      t0 = now();
      dur = ms;
      speed = sp;
      map.triggerRepaint();
      return this;
    },
    remove() {
      map.off("styledata", again);
      if (map.getLayer(layer.id)) map.removeLayer(layer.id);
    },
  };
}
