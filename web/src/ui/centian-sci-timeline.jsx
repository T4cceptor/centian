import { useState, useMemo } from "react";

// Inline stylesheet for the standalone timeline demo.
const STYLES = `
  @import url('https://fonts.googleapis.com/css2?family=Share+Tech+Mono&display=swap');

  .ctv * { box-sizing: border-box; }

  @keyframes travel {
    0%   { top: -60px; opacity: 0; }
    6%   { opacity: 0.9; }
    94%  { opacity: 0.6; }
    100% { top: calc(100% + 60px); opacity: 0; }
  }
  @keyframes breathe {
    0%, 100% { transform: scale(1);    opacity: 0.2; }
    50%       { transform: scale(1.65); opacity: 0.06; }
  }
  @keyframes pop-in {
    from { opacity: 0; transform: scale(0.94) translateY(12px); }
    to   { opacity: 1; transform: scale(1)    translateY(0px); }
  }
  @keyframes sector-scan {
    0%   { transform: translateX(-100%); opacity: 0; }
    20%  { opacity: 0.6; }
    80%  { opacity: 0.6; }
    100% { transform: translateX(100%); opacity: 0; }
  }
  @keyframes blink-cursor {
    0%, 100% { opacity: 1; } 50% { opacity: 0; }
  }
  @keyframes flicker {
    0%, 95%, 100% { opacity: 1; }
    96% { opacity: 0.6; }
    97% { opacity: 1; }
    98% { opacity: 0.5; }
    99% { opacity: 0.9; }
  }
  @keyframes halo-spin {
    from { transform: rotate(0deg); }
    to   { transform: rotate(360deg); }
  }
  @keyframes status-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }

  .node { cursor: pointer; transition: transform 0.18s cubic-bezier(.34,1.56,.64,1); }
  .node:hover { transform: scale(1.08); }
  .node:hover .outer  { animation: breathe 1.1s ease-in-out infinite !important; opacity: 0.38 !important; }
  .node:hover .tag    { border-color: rgba(255,255,255,0.14) !important; background: rgba(255,255,255,0.04) !important; }
  .node:hover .conn   { opacity: 0.55 !important; width: 24px !important; }
  .node:hover .ts     { color: #4a6080 !important; }

  .modal-enter { animation: pop-in 0.22s cubic-bezier(.22,1,.36,1) forwards; }

  .grid-bg {
    background-image:
      radial-gradient(circle, rgba(30,40,80,0.35) 1px, transparent 1px);
    background-size: 28px 28px;
  }
`;

// Color tokens keyed by server or alert state.
const C = {
  centian:    { color: "#a78bfa", bg: "rgba(167,139,250,0.1)",  glow: "rgba(167,139,250,0.6)", dim: "#3b2e6e" },
  shell:      { color: "#fbbf24", bg: "rgba(251,191,36,0.1)",   glow: "rgba(251,191,36,0.6)",  dim: "#6b4f10" },
  filesystem: { color: "#34d399", bg: "rgba(52,211,153,0.1)",   glow: "rgba(52,211,153,0.6)",  dim: "#0e4a35" },
  error:      { color: "#f87171", bg: "rgba(248,113,113,0.12)", glow: "rgba(248,113,113,0.7)", dim: "#5c1e1e" },
  warn:       { color: "#fb923c", bg: "rgba(251,146,60,0.1)",   glow: "rgba(251,146,60,0.55)", dim: "#5c3010" },
};
// Chooses the visual treatment for each processed log entry.
const cv = (entry) => {
  if (entry.isError && !entry.isWarning) return C.error;
  if (entry.isWarning) return C.warn;
  return C[entry.routing?.server_name] || C.centian;
};

// Sample trace data used by the design/demo component.
const RAW_LOG = [
  {"timestamp":"2026-03-20T22:56:09.780Z","request_id":"r-9a34","message_type":"request","routing":{"server_name":"centian"},"tool_call":{"name":"centian.task_list_templates","arguments":{},"result":{"structuredContent":{"templates":[{"id":"python_tdd_demo","name":"Python TDD Demo","stepCount":2}]}}}},
  {"timestamp":"2026-03-20T22:56:11.040Z","request_id":"r-9f20","message_type":"request","routing":{"server_name":"centian"},"tool_call":{"name":"centian.task_register","arguments":{"templateId":"python_tdd_demo","parameters":{"testCommand":"python -m pytest -q","testTarget":"tests/test_mathlib.py","lintCommand":"python -m ruff check .","expectedError":"assert -1 == 5"}},"result":{"structuredContent":{"status":"registered","stepCount":2}}}},
  {"timestamp":"2026-03-20T22:56:12.524Z","request_id":"r-a4ec","message_type":"request","routing":{"server_name":"centian"},"tool_call":{"name":"centian.task_start_step","arguments":{"step":1},"result":{"structuredContent":{"step":1,"stepId":"establish_failing_baseline","stepStatus":"active","steps":[{"id":"establish_failing_baseline","status":"active","step":1},{"id":"implement_fix","status":"pending","step":2}]}}}},
  {"timestamp":"2026-03-20T22:56:14.267Z","request_id":"r-abb9","message_type":"request","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"pwd"}}},
  {"timestamp":"2026-03-20T22:56:14.293Z","request_id":"r-abb9","message_type":"response","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"pwd"},"result":{"content":[{"type":"text","text":"exit_code: 0\nstdout: /opt/centian"}]}}},
  {"timestamp":"2026-03-20T22:56:14.298Z","request_id":"r-abda","message_type":"request","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"python -m pytest -q tests/test_mathlib.py"}}},
  {"timestamp":"2026-03-20T22:56:14.419Z","request_id":"r-abda","message_type":"response","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"python -m pytest -q tests/test_mathlib.py"},"result":{"content":[{"type":"text","text":"exit_code: 4\nERROR: file or directory not found: tests/test_mathlib.py"}]}}},
  {"timestamp":"2026-03-20T22:56:16.544Z","request_id":"r-b49f","message_type":"request","routing":{"server_name":"filesystem"},"tool_call":{"name":"list_allowed_directories","arguments":{}}},
  {"timestamp":"2026-03-20T22:56:16.550Z","request_id":"r-b49f","message_type":"response","routing":{"server_name":"filesystem"},"tool_call":{"name":"list_allowed_directories","arguments":{},"result":{"content":[{"type":"text","text":"Allowed directories:\n/workspace/project"}]}}},
  {"timestamp":"2026-03-20T22:56:18.498Z","request_id":"r-bc41","message_type":"request","routing":{"server_name":"filesystem"},"tool_call":{"name":"list_directory","arguments":{"path":"/workspace/project"}}},
  {"timestamp":"2026-03-20T22:56:18.505Z","request_id":"r-bc41","message_type":"response","routing":{"server_name":"filesystem"},"tool_call":{"name":"list_directory","arguments":{"path":"/workspace/project"},"result":{"content":[{"type":"text","text":"[DIR] tests\n[FILE] mathlib.py\n[FILE] pyproject.toml"}]}}},
  {"timestamp":"2026-03-20T22:56:18.509Z","request_id":"r-bc4d","message_type":"request","routing":{"server_name":"filesystem"},"tool_call":{"name":"directory_tree","arguments":{"path":"/workspace/project"}}},
  {"timestamp":"2026-03-20T22:56:18.519Z","request_id":"r-bc4d","message_type":"response","routing":{"server_name":"filesystem"},"tool_call":{"name":"directory_tree","arguments":{"path":"/workspace/project"},"result":{"content":[{"type":"text","text":"[tree output — /workspace/project]"}]}}},
  {"timestamp":"2026-03-20T22:56:20.666Z","request_id":"r-c4b9","message_type":"request","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"cd /workspace/project && python -m pytest -q tests/test_mathlib.py"}}},
  {"timestamp":"2026-03-20T22:56:20.676Z","request_id":"r-c4b9","message_type":"response","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"cd /workspace/project && python -m pytest -q tests/test_mathlib.py"},"result":{"content":[{"type":"text","text":"Error executing command: spawn cd ENOENT"}],"isError":true}}},
  {"timestamp":"2026-03-20T22:56:20.680Z","request_id":"r-c4c7","message_type":"request","routing":{"server_name":"filesystem"},"tool_call":{"name":"read_multiple_files","arguments":{"paths":["/workspace/project/tests/test_mathlib.py","/workspace/project/mathlib.py"]}}},
  {"timestamp":"2026-03-20T22:56:20.687Z","request_id":"r-c4c7","message_type":"response","routing":{"server_name":"filesystem"},"tool_call":{"name":"read_multiple_files","arguments":{"paths":["/workspace/project/tests/test_mathlib.py","/workspace/project/mathlib.py"]},"result":{"content":[{"type":"text","text":"test_mathlib.py:\nassert add(2,3) == 5\n\nmathlib.py:\nreturn a - b"}]}}},
  {"timestamp":"2026-03-20T22:56:24.162Z","request_id":"r-d262","message_type":"request","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"env PYTHONPATH=/workspace/project python -m pytest -q /workspace/project/tests/test_mathlib.py"}}},
  {"timestamp":"2026-03-20T22:56:24.291Z","request_id":"r-d262","message_type":"response","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"env PYTHONPATH=/workspace/project python -m pytest -q /workspace/project/tests/test_mathlib.py"},"result":{"content":[{"type":"text","text":"exit_code: 1\nFAILED test_add_two_numbers — assert -1 == 5\n1 failed in 0.01s"}]}}},
  {"timestamp":"2026-03-20T22:56:25.291Z","request_id":"r-d6cb","message_type":"request","routing":{"server_name":"centian"},"tool_call":{"name":"centian.task_complete_step","arguments":{"step":1},"result":{"structuredContent":{"step":1,"stepId":"establish_failing_baseline","stepStatus":"passed","steps":[{"id":"establish_failing_baseline","status":"passed","step":1},{"id":"implement_fix","status":"pending","step":2}]}}}},
  {"timestamp":"2026-03-20T22:56:27.332Z","request_id":"r-dec4","message_type":"request","routing":{"server_name":"filesystem"},"tool_call":{"name":"edit_file","arguments":{"path":"/workspace/project/mathlib.py","edits":[{"oldText":"return a - b","newText":"return a + b"}]}}},
  {"timestamp":"2026-03-20T22:56:27.349Z","request_id":"r-dec4","message_type":"response","routing":{"server_name":"filesystem"},"tool_call":{"name":"edit_file","arguments":{"path":"/workspace/project/mathlib.py"},"result":{"content":[{"type":"text","text":"-    return a - b\n+    return a + b"}]}}},
  {"timestamp":"2026-03-20T22:56:30.146Z","request_id":"r-e9c2","message_type":"request","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"env PYTHONPATH=/workspace/project python -m pytest -q /workspace/project/tests/test_mathlib.py"}}},
  {"timestamp":"2026-03-20T22:56:30.267Z","request_id":"r-e9c2","message_type":"response","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"env PYTHONPATH=/workspace/project python -m pytest -q /workspace/project/tests/test_mathlib.py"},"result":{"content":[{"type":"text","text":"exit_code: 0\n1 passed in 0.00s"}]}}},
  {"timestamp":"2026-03-20T22:56:30.269Z","request_id":"r-ea3d","message_type":"request","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"python -m ruff check /workspace/project"}}},
  {"timestamp":"2026-03-20T22:56:30.316Z","request_id":"r-ea3d","message_type":"response","routing":{"server_name":"shell"},"tool_call":{"name":"execute_command","arguments":{"command":"python -m ruff check /workspace/project"},"result":{"content":[{"type":"text","text":"exit_code: 0\nAll checks passed!"}]}}},
  {"timestamp":"2026-03-20T22:56:31.338Z","request_id":"r-ee6b","message_type":"request","routing":{"server_name":"centian"},"tool_call":{"name":"centian.task_start_step","arguments":{"step":2},"result":{"structuredContent":{"step":2,"stepId":"implement_fix","stepStatus":"active","steps":[{"id":"establish_failing_baseline","status":"passed","step":1},{"id":"implement_fix","status":"active","step":2}]}}}},
  {"timestamp":"2026-03-20T22:56:32.227Z","request_id":"r-f1e3","message_type":"request","routing":{"server_name":"centian"},"tool_call":{"name":"centian.task_complete_step","arguments":{"step":2},"result":{"structuredContent":{"step":2,"stepId":"implement_fix","stepStatus":"passed","taskStatus":"completed","steps":[{"id":"establish_failing_baseline","status":"passed","step":1},{"id":"implement_fix","status":"passed","step":2}]}}}},
];

// Detects exchanges that should be treated as hard failures in the demo UI.
function hasError(entry) {
  const r = entry.tool_call?.result;
  if (!r) return false;
  if (r.isError === true) return true;
  const t = r.content?.[0]?.text || "";
  if (/exit_code:\s+[1-9]\d*/.test(t)) return true;
  if (t.includes("ENOENT") || t.startsWith("Error executing")) return true;
  return false;
}
// Separates non-zero exit codes from transport/runtime failures so they can render as warnings.
function isWarning(entry) {
  if (!entry.isError) return false;
  const r = entry.result || entry.tool_call?.result;
  if (r?.isError) return false;
  const t = r?.content?.[0]?.text || "";
  if (t.includes("ENOENT") || t.startsWith("Error executing")) return false;
  return /exit_code:\s+[1-9]\d*/.test(t);
}

// Pairs requests and responses, then groups them into setup/step phases for rendering.
function processLog(log) {
  const paired = [];
  const pending = {};
  for (const e of log) {
    const srv = e.routing?.server_name;
    if (srv === "centian") {
      paired.push({ ...e, isCentian: true, isError: false, isWarning: false, duration: 0 });
    } else if (e.message_type === "request") {
      pending[e.request_id] = e;
    } else if (e.message_type === "response") {
      const req = pending[e.request_id];
      if (req) {
        const err = hasError(e);
        const entry = {
          ...req, isCentian: false,
          responseTimestamp: e.timestamp,
          duration: new Date(e.timestamp) - new Date(req.timestamp),
          result: e.tool_call?.result,
          isError: err,
        };
        entry.isWarning = isWarning(entry);
        paired.push(entry);
        delete pending[e.request_id];
      }
    }
  }

  // Build phases
  const phases = [];
  let cur = { type: "setup", label: "Initialisation", stepNum: 0, stepId: "setup", status: "info", entries: [] };
  phases.push(cur);

  for (const p of paired) {
    const name = p.tool_call?.name;
    if (p.isCentian && name === "centian.task_start_step") {
      const sc = p.tool_call?.result?.structuredContent;
      cur = { type: "step", label: sc?.stepId?.replace(/_/g, " "), stepNum: sc?.step, stepId: sc?.stepId, status: "active", startTime: p.timestamp, entries: [] };
      phases.push(cur);
      cur.entries.push(p);
    } else if (p.isCentian && name === "centian.task_complete_step") {
      const sc = p.tool_call?.result?.structuredContent;
      if (cur.type === "step") { cur.status = sc?.stepStatus || "passed"; cur.endTime = p.timestamp; }
      cur.entries.push(p);
    } else {
      cur.entries.push(p);
    }
  }

  // Reassign orphaned downstream calls to the following step when the agent logged them late.
  const steps = phases.filter(p => p.type === "step");
  for (let i = 0; i < steps.length; i++) {
    const s = steps[i];
    if (s.entries.filter(e => !e.isCentian).length === 0 && i > 0) {
      const prev = steps[i - 1];
      const ci = prev.entries.findIndex(e => e.tool_call?.name === "centian.task_complete_step");
      if (ci >= 0) {
        const stolen = prev.entries.splice(ci + 1);
        s.entries = [s.entries[0], ...stolen, ...s.entries.slice(1)];
      }
    }
  }

  const t0 = Math.min(...paired.map(p => new Date(p.timestamp).getTime()));
  const tEnd = Math.max(...paired.map(p => new Date(p.responseTimestamp || p.timestamp).getTime()));
  return { phases, t0, totalMs: tEnd - t0 };
}

// Draws a different node shape for each server family.
function NodeShape({ server, size, color }) {
  if (server === "centian") return (
    <div style={{ width: size, height: size, clipPath: "polygon(50% 0%,100% 25%,100% 75%,50% 100%,0% 75%,0% 25%)", background: color }} />
  );
  if (server === "filesystem") return (
    <div style={{ width: size * 0.76, height: size * 0.76, background: color, transform: "rotate(45deg)", borderRadius: 2 }} />
  );
  return <div style={{ width: size, height: size, borderRadius: "50%", background: color }} />;
}

// Shortens tool names for the compact node label.
function shortName(entry) {
  return (entry.tool_call?.name || "")
    .replace("centian.", "")
    .replace("shell___", "").replace("filesystem___", "")
    .replace("execute_command", "exec");
}
// Chooses the most useful short secondary label for a node.
function subLabel(entry) {
  const a = entry.tool_call?.arguments;
  if (a?.command) return (a.command.length > 46 ? a.command.slice(0, 46) + "…" : a.command);
  if (a?.path) return a.path.split("/").slice(-2).join("/");
  if (a?.paths) return a.paths.map(p => p.split("/").pop()).join(", ");
  if (a?.templateId) return a.templateId;
  if (a?.step !== undefined) return `step ${a.step}`;
  return null;
}
// Formats demo timestamps as dense trace-style clock strings.
function fmtTs(ts) {
  return new Date(ts).toLocaleTimeString("en-US", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit", fractionalSecondDigits: 3 });
}

// Renders the phase divider between setup and task steps.
function SectorDivider({ phase }) {
  const isStep = phase.type === "step";
  const statusColor = phase.status === "passed" ? "#34d399" : phase.status === "failed" ? "#f87171" : "#a78bfa";
  const stepColors = [null, "#a78bfa", "#34d399", "#fbbf24"];
  const accentColor = isStep ? (stepColors[phase.stepNum] || "#a78bfa") : "#2d3a5a";

  return (
    <div style={{ position: "relative", margin: "10px 0 4px", padding: "0 0 0 120px" }}>
      {/* Full-width horizontal rule with glow */}
      <div style={{ position: "relative", height: 1, background: `linear-gradient(to right, transparent, ${accentColor}44, ${accentColor}22, transparent)`, marginBottom: 8 }}>
        <div style={{ position: "absolute", inset: 0, background: `linear-gradient(to right, transparent, ${accentColor}, transparent)`, opacity: 0.15 }} />
        {/* Scan sweep */}
        <div style={{
          position: "absolute", inset: 0,
          background: `linear-gradient(to right, transparent, ${accentColor}, transparent)`,
          animation: "sector-scan 4s ease-in-out infinite",
          animationDelay: `${phase.stepNum * 1.3}s`,
        }} />
      </div>

      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        {/* Corner bracket */}
        <div style={{
          width: 10, height: 10, flexShrink: 0,
          borderTop: `1px solid ${accentColor}`,
          borderLeft: `1px solid ${accentColor}`,
          opacity: 0.7,
        }} />

        {isStep ? (
          <>
            <span style={{ fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 10, color: accentColor, letterSpacing: "0.2em", textTransform: "uppercase", opacity: 0.8 }}>
              SECTOR {String(phase.stepNum).padStart(2, "0")}
            </span>
            <span style={{ fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 11, color: "#4a5f80", letterSpacing: "0.08em" }}>
              {phase.label}
            </span>
            <div style={{ flex: 1, height: 1, background: `linear-gradient(to right, ${accentColor}20, transparent)` }} />
            <span style={{
              fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 9,
              color: statusColor, letterSpacing: "0.15em", textTransform: "uppercase",
              padding: "2px 8px", border: `1px solid ${statusColor}40`,
              borderRadius: 2, background: `${statusColor}0d`,
              animation: phase.status === "active" ? "status-pulse 2s ease-in-out infinite" : "none",
            }}>
              ● {phase.status}
            </span>
          </>
        ) : (
          <span style={{ fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 10, color: "#2d3a5a", letterSpacing: "0.18em" }}>
            INIT SEQUENCE
          </span>
        )}

        <div style={{
          width: 10, height: 10, flexShrink: 0,
          borderTop: `1px solid ${accentColor}`,
          borderRight: `1px solid ${accentColor}`,
          opacity: 0.7,
        }} />
      </div>
    </div>
  );
}

// Renders one timeline row and opens the modal when selected.
function EventNode({ entry, t0, totalMs, onClick }) {
  const server = entry.routing?.server_name;
  const vc = cv(entry);
  const name = shortName(entry);
  const sub = subLabel(entry);
  const pct = Math.round(((new Date(entry.timestamp).getTime() - t0) / totalMs) * 100);

  // Mirror the inner node shape so the surrounding halo stays visually aligned.
  const outerStyle = server === "centian"
    ? { clipPath: "polygon(50% 0%,100% 25%,100% 75%,50% 100%,0% 75%,0% 25%)" }
    : server === "filesystem"
    ? { transform: "rotate(45deg)", borderRadius: 3 }
    : { borderRadius: "50%" };

  return (
    <div
      className="node"
      onClick={() => onClick(entry)}
      style={{ display: "flex", alignItems: "center", minHeight: 50, position: "relative" }}
    >
      {/* Timestamp */}
      <div className="ts" style={{
        width: 76, textAlign: "right", paddingRight: 0, flexShrink: 0,
        fontFamily: "'Share Tech Mono', 'Courier New', monospace",
        fontSize: 10, color: "#1e2a42", letterSpacing: "0.02em",
        transition: "color 0.2s",
      }}>
        {fmtTs(entry.timestamp)}
      </div>

      {/* Timeline column with bubble */}
      <div style={{ width: 44, display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0, position: "relative", zIndex: 2 }}>
        {/* Outer halo */}
        <div className="outer" style={{
          position: "absolute",
          width: 40, height: 40,
          background: vc.bg,
          ...outerStyle,
          opacity: 0.15,
          transition: "opacity 0.2s",
        }} />
        {/* Middle ring */}
        <div style={{
          position: "absolute",
          width: 24, height: 24,
          border: `1px solid ${vc.color}55`,
          boxShadow: `0 0 10px ${vc.glow}, inset 0 0 8px ${vc.bg}`,
          ...outerStyle,
        }} />
        {/* Core shape */}
        <NodeShape server={server} size={12} color={vc.color} />
        {/* Progress pip marks that the row belongs to the active session rail. */}
        <div style={{
          position: "absolute", bottom: -2, right: 0,
          width: 4, height: 4, borderRadius: "50%",
          background: "#1a2540",
          boxShadow: `0 0 0 1px #0d1525`,
        }} />
      </div>

      {/* Connector + label tag */}
      <div style={{ flex: 1, display: "flex", alignItems: "center", paddingLeft: 0 }}>
        {/* Connector line */}
        <div className="conn" style={{
          width: 16, height: 1, flexShrink: 0,
          background: `linear-gradient(to right, ${vc.color}, ${vc.color}44)`,
          opacity: 0.2, transition: "all 0.2s",
        }} />

        {/* Label pill */}
        <div className="tag" style={{
          border: `1px solid ${vc.color}18`,
          borderLeft: `2px solid ${vc.color}99`,
          borderRadius: "0 4px 4px 0",
          padding: "5px 12px 5px 10px",
          background: "transparent",
          transition: "all 0.18s",
          flex: 1,
          maxWidth: 340,
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "nowrap" }}>
            {/* Server dot */}
            <div style={{
              width: 5, height: 5, borderRadius: "50%", flexShrink: 0,
              background: vc.color,
              boxShadow: `0 0 6px ${vc.glow}`,
            }} />
            {/* Server name */}
            <span style={{
              fontFamily: "'Share Tech Mono', 'Courier New', monospace",
              fontSize: 9, color: vc.color, letterSpacing: "0.12em",
              textTransform: "uppercase", opacity: 0.75, flexShrink: 0,
            }}>
              {server}
            </span>
            {/* Tool name */}
            <span style={{
              fontFamily: "'Share Tech Mono', 'Courier New', monospace",
              fontSize: 12, color: "#8ba4c8", fontWeight: "normal",
              overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
            }}>
              {name}
            </span>
            {/* Error / warn badge */}
            {entry.isError && !entry.isWarning && (
              <span style={{ fontSize: 8, color: "#f87171", background: "#f871710d", border: "1px solid #f8717133", padding: "1px 5px", borderRadius: 2, letterSpacing: "0.12em", flexShrink: 0 }}>ERR</span>
            )}
            {entry.isWarning && (
              <span style={{ fontSize: 8, color: "#fb923c", background: "#fb923c0d", border: "1px solid #fb923c33", padding: "1px 5px", borderRadius: 2, letterSpacing: "0.12em", flexShrink: 0 }}>
                EXIT {(entry.result?.content?.[0]?.text || "").match(/exit_code:\s+(\d+)/)?.[1] || "!0"}
              </span>
            )}
            {/* Duration */}
            {entry.duration > 0 && (
              <span style={{ fontFamily: "monospace", fontSize: 9, color: "#1e2a42", marginLeft: "auto", flexShrink: 0, paddingRight: 2 }}>
                {entry.duration}ms
              </span>
            )}
          </div>
          {/* Subtext */}
          {sub && (
            <div style={{
              fontFamily: "'Share Tech Mono', 'Courier New', monospace",
              fontSize: 10, color: "#2a3d5e", marginTop: 3,
              overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
              maxWidth: 290,
            }}>
              {sub}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// Shows the full request/response payload for the selected node.
function DetailModal({ entry, onClose }) {
  if (!entry) return null;
  const server = entry.routing?.server_name;
  const vc = cv(entry);
  const args = entry.tool_call?.arguments;
  const result = entry.result || entry.tool_call?.result;
  const resultText = result?.content?.[0]?.text || JSON.stringify(result?.structuredContent, null, 2) || "";

  return (
    <div
      onClick={onClose}
      style={{
        position: "fixed", inset: 0, zIndex: 200,
        background: "rgba(1,2,14,0.85)",
        display: "flex", alignItems: "center", justifyContent: "center",
        backdropFilter: "blur(6px)",
      }}
    >
      <div
        className="modal-enter"
        onClick={e => e.stopPropagation()}
        style={{
          width: "min(620px, 92vw)",
          background: "#070d1f",
          border: `1px solid ${vc.color}33`,
          borderRadius: 2,
          boxShadow: `0 0 60px ${vc.glow}22, 0 0 120px rgba(0,0,0,0.9), inset 0 0 40px rgba(0,0,0,0.5)`,
          overflow: "hidden",
        }}
      >
        {/* HUD top bar */}
        <div style={{
          background: `linear-gradient(135deg, ${vc.bg}, transparent)`,
          borderBottom: `1px solid ${vc.color}22`,
          padding: "12px 20px",
          display: "flex", alignItems: "center", gap: 12,
        }}>
          {/* Corner brackets */}
          <div style={{ position: "relative", width: 16, height: 16, flexShrink: 0 }}>
            <div style={{ position: "absolute", top: 0, left: 0, width: 8, height: 8, borderTop: `2px solid ${vc.color}`, borderLeft: `2px solid ${vc.color}` }} />
          </div>
          <NodeShape server={server} size={14} color={vc.color} />
          <span style={{ fontFamily: "'Share Tech Mono','Courier New',monospace", fontSize: 13, color: vc.color, letterSpacing: "0.12em" }}>
            {server?.toUpperCase()} // {shortName(entry)}
          </span>
          <div style={{ flex: 1 }} />
          <span style={{ fontFamily: "'Share Tech Mono','Courier New',monospace", fontSize: 10, color: "#2d3a5a" }}>
            {fmtTs(entry.timestamp)}
            {entry.duration > 0 && ` · ${entry.duration}ms`}
          </span>
          {/* Corner bracket right */}
          <div style={{ position: "relative", width: 16, height: 16, flexShrink: 0 }}>
            <div style={{ position: "absolute", top: 0, right: 0, width: 8, height: 8, borderTop: `2px solid ${vc.color}`, borderRight: `2px solid ${vc.color}` }} />
          </div>
          <button onClick={onClose} style={{ background: "none", border: "none", color: "#2d3a5a", fontSize: 16, cursor: "pointer", padding: "0 0 0 4px", lineHeight: 1 }}>×</button>
        </div>

        {/* Content */}
        <div style={{ padding: "16px 20px 20px", display: "flex", flexDirection: "column", gap: 14, maxHeight: "70vh", overflowY: "auto" }}>
          {/* Request ID */}
          <div style={{ fontFamily: "'Share Tech Mono','Courier New',monospace", fontSize: 9, color: "#1e2a42", letterSpacing: "0.12em" }}>
            REQ · {entry.request_id}
          </div>

          {/* Arguments */}
          {args && Object.keys(args).length > 0 && (
            <div>
              <div style={{ fontFamily: "'Share Tech Mono','Courier New',monospace", fontSize: 9, color: vc.color, letterSpacing: "0.18em", marginBottom: 8, opacity: 0.7 }}>
                ▸ ARGUMENTS
              </div>
              <pre style={{
                background: "#030710",
                border: `1px solid ${vc.color}18`,
                borderLeft: `2px solid ${vc.color}55`,
                borderRadius: 2,
                padding: "10px 14px",
                fontFamily: "'Share Tech Mono','Courier New',monospace",
                fontSize: 11, color: "#8ba4c8", lineHeight: 1.7,
                overflow: "auto", maxHeight: 180, margin: 0,
              }}>
                {JSON.stringify(args, null, 2)}
              </pre>
            </div>
          )}

          {/* Result */}
          {resultText && (
            <div>
              <div style={{ fontFamily: "'Share Tech Mono','Courier New',monospace", fontSize: 9, color: vc.color, letterSpacing: "0.18em", marginBottom: 8, opacity: 0.7 }}>
                ▸ RESPONSE
              </div>
              <pre style={{
                background: "#030710",
                border: `1px solid ${(entry.isError && !entry.isWarning) ? "#f8717133" : vc.color + "18"}`,
                borderLeft: `2px solid ${vc.color}55`,
                borderRadius: 2,
                padding: "10px 14px",
                fontFamily: "'Share Tech Mono','Courier New',monospace",
                fontSize: 11, lineHeight: 1.7, overflow: "auto", maxHeight: 240, margin: 0,
                color: (entry.isError && !entry.isWarning) ? "#f87171" : entry.isWarning ? "#fb923c" : "#34d399",
              }}>
                {resultText}
              </pre>
            </div>
          )}
        </div>

        {/* Bottom corner accents */}
        <div style={{ height: 4, background: `linear-gradient(to right, ${vc.color}33, transparent, ${vc.color}33)` }} />
      </div>
    </div>
  );
}

// Decorative tracer that can be layered over the vertical timeline rail.
function PulseTracer({ color }) {
  return (
    <div style={{ position: "absolute", top: 0, left: "50%", transform: "translateX(-50%)", width: 2, height: "100%", pointerEvents: "none", overflow: "hidden" }}>
      <div style={{
        position: "absolute", left: "50%", transform: "translateX(-50%)",
        width: 6, height: 60,
        background: `linear-gradient(to bottom, transparent, ${color}, transparent)`,
        boxShadow: `0 0 12px ${color}`,
        animation: "travel 6s ease-in-out infinite",
        animationDelay: "1s",
      }} />
    </div>
  );
}

// Standalone showcase component for the sci-fi trace timeline treatment.
export default function CentianSciTimeline() {
  const { phases, t0, totalMs } = useMemo(() => processLog(RAW_LOG), []);
  const [selected, setSelected] = useState(null);

  // Flatten phases into a single render list so dividers and events share one vertical rail.
  const rows = [];
  for (const phase of phases) {
    rows.push({ type: "sector", phase });
    for (const entry of phase.entries) {
      rows.push({ type: "event", entry, phase });
    }
  }

  // Summary stats drive the compact HUD copy in the header.
  const allEntries = phases.flatMap(p => p.entries);
  const errCount = allEntries.filter(e => e.isError && !e.isWarning).length;
  const warnCount = allEntries.filter(e => e.isWarning).length;
  const totalCalls = allEntries.length;

  return (
    <div className="ctv" style={{
      background: "#020210",
      minHeight: "100vh",
      color: "#8ba4c8",
      fontFamily: "'Share Tech Mono','Courier New',monospace",
      position: "relative",
      overflow: "hidden",
    }}>
      <style dangerouslySetInnerHTML={{ __html: STYLES }} />

      {/* Dot grid background */}
      <div className="grid-bg" style={{ position: "fixed", inset: 0, pointerEvents: "none", zIndex: 0 }} />

      {/* Radial depth vignette */}
      <div style={{
        position: "fixed", inset: 0, pointerEvents: "none", zIndex: 0,
        background: "radial-gradient(ellipse at 50% 30%, transparent 40%, rgba(1,1,16,0.7) 100%)",
      }} />

      {/* Content */}
      <div style={{ position: "relative", zIndex: 1, maxWidth: 720, margin: "0 auto", padding: "32px 24px 80px" }}>

        {/* ── Header HUD ── */}
        <div style={{ marginBottom: 36 }}>
          {/* Top corner bracket decoration */}
          <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 14 }}>
            <div style={{ width: 24, height: 24, borderTop: "1px solid #a78bfa44", borderLeft: "1px solid #a78bfa44" }} />
            <div style={{ width: 24, height: 24, borderTop: "1px solid #a78bfa44", borderRight: "1px solid #a78bfa44" }} />
          </div>

          <div style={{ textAlign: "center" }}>
            <div style={{ fontSize: 10, color: "#2d3a5a", letterSpacing: "0.35em", marginBottom: 10, animation: "flicker 8s ease-in-out infinite" }}>
              CENTIAN TRACE SYSTEM · SESSION LOG ACTIVE
            </div>
            <div style={{ fontSize: 22, color: "#c4d4f0", letterSpacing: "0.08em", marginBottom: 6 }}>
              python_tdd_demo
            </div>
            <div style={{ fontSize: 10, color: "#2d3a5a", letterSpacing: "0.15em" }}>
              {fmtTs(new Date(t0).toISOString())}
              <span style={{ margin: "0 12px", color: "#1a2540" }}>·</span>
              {(totalMs / 1000).toFixed(1)}s
              <span style={{ margin: "0 12px", color: "#1a2540" }}>·</span>
              {totalCalls} calls
              {errCount > 0 && <><span style={{ margin: "0 12px", color: "#1a2540" }}>·</span><span style={{ color: "#f87171" }}>{errCount} err</span></>}
              {warnCount > 0 && <><span style={{ margin: "0 12px", color: "#1a2540" }}>·</span><span style={{ color: "#fb923c" }}>{warnCount} warn</span></>}
            </div>
          </div>

          {/* Server legend */}
          <div style={{ display: "flex", justifyContent: "center", gap: 24, marginTop: 18 }}>
            {Object.entries({ centian: "◆ hexagon", shell: "● circle", filesystem: "◈ diamond" }).map(([srv, shape]) => (
              <div key={srv} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 9, color: C[srv].color, opacity: 0.6, letterSpacing: "0.1em" }}>
                <NodeShape server={srv} size={10} color={C[srv].color} />
                <span>{srv}</span>
              </div>
            ))}
          </div>

          {/* Bottom line */}
          <div style={{ marginTop: 18, height: 1, background: "linear-gradient(to right, transparent, #a78bfa33, transparent)" }} />
        </div>

        {/* ── Timeline ── */}
        <div style={{ position: "relative" }}>

          {/* Vertical timeline line */}
          <div style={{
            position: "absolute",
            left: 119, top: 0, bottom: 0, width: 1,
            background: "linear-gradient(to bottom, transparent, #1e2a5a44 5%, #1e2a5a44 95%, transparent)",
            zIndex: 1,
          }} />
          {/* Glow line */}
          <div style={{
            position: "absolute",
            left: 119, top: 0, bottom: 0, width: 1,
            background: "linear-gradient(to bottom, transparent, #a78bfa18 10%, #a78bfa10 90%, transparent)",
            boxShadow: "0 0 8px rgba(167,139,250,0.08)",
            zIndex: 1,
          }} />
          {/* Animated pulse tracer */}
          <div style={{ position: "absolute", left: 116, top: 0, bottom: 0, width: 6, pointerEvents: "none", overflow: "hidden", zIndex: 2 }}>
            <div style={{
              position: "absolute", left: 0, width: 6, height: 80,
              background: "linear-gradient(to bottom, transparent, rgba(167,139,250,0.8), rgba(167,139,250,0.3), transparent)",
              boxShadow: "0 0 12px rgba(167,139,250,0.6)",
              animation: "travel 7s ease-in-out infinite",
              animationDelay: "0.5s",
            }} />
          </div>

          {/* Rows */}
          {rows.map((row, i) => {
            if (row.type === "sector") {
              return <SectorDivider key={`s-${i}`} phase={row.phase} />;
            }
            return (
              <EventNode
                key={`e-${row.entry.request_id}-${i}`}
                entry={row.entry}
                t0={t0}
                totalMs={totalMs}
                onClick={setSelected}
              />
            );
          })}

          {/* End cap */}
          <div style={{ display: "flex", alignItems: "center", paddingLeft: 76, marginTop: 8, gap: 12 }}>
            <div style={{ width: 44, display: "flex", justifyContent: "center" }}>
              <div style={{ width: 8, height: 8, background: "#34d399", clipPath: "polygon(50% 0%,100% 50%,50% 100%,0% 50%)", boxShadow: "0 0 10px rgba(52,211,153,0.6)" }} />
            </div>
            <span style={{ fontSize: 9, color: "#34d399", letterSpacing: "0.25em", opacity: 0.6 }}>SESSION COMPLETE</span>
          </div>
        </div>
      </div>

      {/* Detail modal */}
      {selected && <DetailModal entry={selected} onClose={() => setSelected(null)} />}
    </div>
  );
}
