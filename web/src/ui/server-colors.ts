export type ColorToken = {
  color: string;
  bg: string;
  glow: string;
  dim: string;
};

const FALLBACK_PALETTE = [
  "#a78bfa",
  "#fbbf24",
  "#34d399",
  "#60a5fa",
  "#fb7185",
  "#22d3ee",
  "#f97316",
  "#4ade80",
];

export const KNOWN_SERVER_COLORS: Record<string, ColorToken> = {
  centian: { color: "#a78bfa", bg: "rgba(167,139,250,0.1)", glow: "rgba(167,139,250,0.6)", dim: "#3b2e6e" },
  shell: { color: "#fbbf24", bg: "rgba(251,191,36,0.1)", glow: "rgba(251,191,36,0.6)", dim: "#6b4f10" },
  filesystem: { color: "#34d399", bg: "rgba(52,211,153,0.1)", glow: "rgba(52,211,153,0.6)", dim: "#0e4a35" },
  error: { color: "#f87171", bg: "rgba(248,113,113,0.12)", glow: "rgba(248,113,113,0.7)", dim: "#5c1e1e" },
};

export function hashServerName(serverName: string): number {
  let hash = 0;
  for (let index = 0; index < serverName.length; index += 1) {
    hash = (hash * 31 + serverName.charCodeAt(index)) >>> 0;
  }
  return hash;
}

export function getServerAccentColor(serverName: string): string {
  return FALLBACK_PALETTE[hashServerName(serverName) % FALLBACK_PALETTE.length];
}

export function getServerColorToken(serverName: string): ColorToken {
  const known = KNOWN_SERVER_COLORS[serverName];
  if (known) {
    return known;
  }

  const color = getServerAccentColor(serverName);
  return {
    color,
    bg: `${color}1a`,
    glow: `${color}99`,
    dim: `${color}44`,
  };
}
