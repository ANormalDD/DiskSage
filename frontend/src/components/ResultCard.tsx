import { useState } from "react";
import { formatBytes } from "../lib/format";
import type { Recommendation } from "../lib/types";

type Props = {
  item: Recommendation;
  checked: boolean;
  onToggle: (path: string) => void;
};

export default function ResultCard({ item, checked, onToggle }: Props) {
  const [copied, setCopied] = useState(false);
  
  const handleCopy = () => {
    navigator.clipboard.writeText(item.command || "");
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const isCommand = item.clean_method === "command";

  return (
    <article className={`item badge-${item.category}`}>
      {isCommand ? (
        <button 
          className="ghost" 
          onClick={handleCopy} 
          style={{ padding: "4px 8px", fontSize: "12px", whiteSpace: "nowrap" }}
        >
          {copied ? "已复制" : "复制指令"}
        </button>
      ) : (
        <input
          type="checkbox"
          checked={checked}
          onChange={() => onToggle(item.path)}
          disabled={item.category === "review"}
        />
      )}
      <div className="item-body">
        <div className="item-path" title={item.path}>{item.path}</div>
        <small className="item-desc">
          {item.reason} | 风险: {item.risk}
          {isCommand && item.command && ` | 指令: ${item.command}`}
        </small>
      </div>
      <strong className="item-size">{formatBytes(item.size)}</strong>
    </article>
  );
}
