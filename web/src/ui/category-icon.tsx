import type { InterventionCategory } from "../api/activity";

// Inline category icons matching the design mockup (no icon-library dependency).
export function CategoryIcon({ category }: { category: InterventionCategory }) {
  const common = {
    width: 12,
    height: 12,
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.7,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": true,
  };
  switch (category) {
    case "security":
      return (
        <svg {...common}>
          <path d="M12 2.5 19.5 5.3V11c0 5-3.4 8.3-7.5 10.2C7.9 19.3 4.5 16 4.5 11V5.3L12 2.5Z" />
        </svg>
      );
    case "policy":
      return (
        <svg {...common}>
          <path d="M6 3h8l4 4v14H6z" />
          <path d="M14 3v4h4" />
          <path d="M9 12h6M9 16h6" />
        </svg>
      );
    case "risk":
      return (
        <svg {...common}>
          <path d="M12 3 22 20H2L12 3Z" />
          <path d="M12 10v4.5M12 17.4h.01" />
        </svg>
      );
    case "quality":
      return (
        <svg {...common}>
          <circle cx="10.5" cy="10.5" r="6.5" />
          <path d="M15.5 15.5 21 21" />
        </svg>
      );
    case "compliance":
      return (
        <svg {...common}>
          <circle cx="12" cy="12" r="9" />
          <path d="M8 12.3l2.7 2.7L16 9.5" />
        </svg>
      );
    default:
      return null;
  }
}
