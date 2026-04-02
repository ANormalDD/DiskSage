import { formatBytes } from "../lib/format";
import type { Recommendation } from "../lib/types";

type Props = {
  item: Recommendation;
  checked: boolean;
  onToggle: (path: string) => void;
};

export default function ResultCard({ item, checked, onToggle }: Props) {
  return (
    <article className={`item badge-${item.category}`}>
      <input
        type="checkbox"
        checked={checked}
        onChange={() => onToggle(item.path)}
        disabled={item.category === "review"}
      />
      <div>
        <div>{item.path}</div>
        <small>
          {item.reason} | 风险: {item.risk}
        </small>
      </div>
      <strong>{formatBytes(item.size)}</strong>
    </article>
  );
}
