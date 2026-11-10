// Renders docs/hero.gif frame by index so output is deterministic: a short
// family scribble, one stroke settles into the waterline, then the boat rocks
// on the water.
// The boat and the waves are the same traced path, split at the waterline by
// two clip paths so they can move independently.
//
// Run from the repo root (needs ffmpeg on PATH):
//   npx -y -p playwright@1.63.0 node scripts/hero-gif.mjs
/* global window, document */
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, statSync } from "node:fs";
import { createRequire } from "node:module";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "..");
const WIDTH = 480;
const HEIGHT = 240;
const FPS = 15;
const PLUM = "#210f27";
const LILAC = "#ab7ad8";
const ROCK = 60; // one full rock cycle, 4s, loops seamlessly

const binDirs = (process.env.PATH ?? "").split(":").filter((d) => d.endsWith("node_modules/.bin"));
const bases = [process.cwd(), ...binDirs.map((d) => join(d, "..", ".."))];
const require = createRequire(join(root, "noop.js"));
const { chromium } = require(require.resolve("playwright", { paths: bases }));

const scribble = readFileSync(join(root, "docs/brand/scribble.svg"), "utf8");
const paths = readFileSync(join(root, "docs/brand/boat-paths.svg.frag"), "utf8");

// Waterline split: the waves keep everything below y 1052, so they read whole
// before the boat arrives; the boat also carries the two hull feet that dip
// into the water, drawn over the same strokes once it is visible.
const feet = "M285 1052H350V1095H285Z M880 1052H950V1095H880Z";
const boatSvg = `<svg viewBox="0 0 1254 1254" width="200" height="200" fill="${LILAC}" stroke="${LILAC}" stroke-width="180" stroke-linejoin="round">
  <defs>
    <clipPath id="above"><path d="M0 0H1254V1052H0Z ${feet}"/></clipPath>
    <clipPath id="below"><path d="M0 1052H1254V1254H0Z"/></clipPath>
    <clipPath id="wipe"><rect id="wiperect" x="0" y="0" width="0" height="1254"/></clipPath>
  </defs>
  <g transform="translate(627 640) scale(0.84) translate(-627 -600)">
    <g id="waves" clip-path="url(#wipe)"><g clip-path="url(#below)">${paths}</g></g>
    <g id="boat"><g clip-path="url(#above)">${paths}</g></g>
  </g>
</svg>`;

const page = (mode) => `<!doctype html>
<html><head><style>
  html, body { margin: 0; width: ${WIDTH}px; height: ${HEIGHT}px; background: ${PLUM}; overflow: hidden; }
  #scribble svg { position: absolute; inset: 0; width: 100%; height: 100%; }
  #tile { position: absolute; left: 50%; top: 50%; width: 212px; height: 212px; border-radius: 40px;
          background: ${PLUM}; transform: translate(-50%, -50%); opacity: 0; }
  #mark { position: absolute; left: 50%; top: 50%; width: 200px; height: 200px; transform: translate(-50%, -50%); }
  #boat { opacity: 0; }
</style></head><body>
  ${mode === "hybrid" ? `<div id="scribble">${scribble}</div><div id="tile"></div>` : ""}
  <div id="mark">${boatSvg}</div>
  <script>
    const MODE = ${JSON.stringify(mode)};
    const clamp = (x) => Math.min(1, Math.max(0, x));
    const ease = (x) => 1 - Math.pow(1 - clamp(x), 3);
    const smooth = (x) => { x = clamp(x); return x * x * (3 - 2 * x); };
    const sp = [...document.querySelectorAll("#scribble path")];
    sp.forEach((p) => { p.setAttribute("pathLength", "1"); p.style.strokeDasharray = "1 1"; });
    // Timeline, in frames.
    const T = MODE === "hybrid"
      ? { draw: 18, tile: [12, 20], wave: [16, 30], boat: [26, 38], rock: 38 }
      : { draw: 0, tile: [0, 0], wave: [0, 16], boat: [10, 24], rock: 24 };
    window.TOTAL = T.rock + ${ROCK} * 2;
    const waves = document.getElementById("waves");
    const boat = document.getElementById("boat");
    const wipe = document.getElementById("wiperect");
    window.render = (f) => {
      if (MODE === "hybrid") {
        sp.forEach((p, i) => {
          const start = (i / sp.length) * 0.5;
          p.style.strokeDashoffset = String(1 - ease((f / T.draw - start) / 0.5));
        });
        const t = smooth((f - T.tile[0]) / (T.tile[1] - T.tile[0]));
        document.getElementById("tile").style.opacity = String(t);
        document.getElementById("scribble").style.opacity = String(1 - 0.55 * t);
      }
      wipe.setAttribute("width", String(1254 * ease((f - T.wave[0]) / (T.wave[1] - T.wave[0]))));
      const b = ease((f - T.boat[0]) / (T.boat[1] - T.boat[0]));
      // Rocking starts from rest and ramps in over the first half cycle so the
      // settle has no jolt; the swell keeps moving the whole time.
      const ph = 2 * Math.PI * (f - T.rock) / ${ROCK};
      const amp = smooth((f - T.rock + 10) / 30);
      const swell = Math.sin(2 * Math.PI * f / ${ROCK});
      const rot = 2.6 * amp * Math.sin(ph);
      const heave = 9 * amp * Math.sin(ph + 1.2) - 30 * (1 - b);
      boat.style.opacity = String(b);
      boat.setAttribute("transform",
        "translate(0 " + (-heave) + ") rotate(" + rot + " 627 1062)");
      waves.setAttribute("transform", "translate(" + (8 * swell) + " " + (5 * Math.sin(2 * Math.PI * f / ${ROCK} + 1.2)) + ")");
    };
  </script>
</body></html>`;

const browser = await chromium.launch();
for (const mode of ["hybrid"]) {
  const out = join(root, "docs/hero.gif");
  const dir = mkdtempSync(join(tmpdir(), `faux3-${mode}-`));
  try {
    const p = await browser.newPage({ viewport: { width: WIDTH, height: HEIGHT } });
    await p.setContent(page(mode));
    const total = await p.evaluate(() => window.TOTAL);
    for (let i = 0; i < total; i++) {
      await p.evaluate((f) => window.render(f), i);
      await p.screenshot({ path: join(dir, `f${String(i).padStart(4, "0")}.png`) });
    }
    await p.close();
    const input = ["-framerate", String(FPS), "-i", join(dir, "f%04d.png")];
    const palette = join(dir, "palette.png");
    execFileSync("ffmpeg", ["-v", "error", "-y", ...input, "-vf", "palettegen=stats_mode=diff:max_colors=64", palette]);
    execFileSync("ffmpeg", ["-v", "error", "-y", ...input, "-i", palette,
      "-lavfi", "paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle", "-loop", "0", out]);
    console.log(`wrote ${out}: ${total} frames, ${statSync(out).size} bytes`);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}
await browser.close();
