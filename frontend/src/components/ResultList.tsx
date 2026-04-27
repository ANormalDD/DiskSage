import ResultCard from "./ResultCard";
import type { CleanCategory, Recommendation } from "../lib/types";
import { formatBytes, titleCaseCategory } from "../lib/format";

type Props = {
  recommendations: Recommendation[];
  selected: Record<string, boolean>;
  onToggle: (path: string) => void;
};

const categories: CleanCategory[] = ["safe", "confirm", "manual", "review"];

export default function ResultList({ recommendations, selected, onToggle }: Props) {
  return (
    <div className="grid-two">
      {categories.map((category) => {
        const rows = recommendations.filter((row) => row.category === category).sort((a, b) => b.size - a.size);
        const total = rows.reduce((sum, row) => sum + row.size, 0);
        return (
          <section className="category" key={category}>
            <h3>
              {titleCaseCategory(category)} ({rows.length}) - {formatBytes(total)}
            </h3>
            <div className="category-list">
              {rows.length === 0 && <p>暂无项目</p>}
              {rows.map((item) => (
                <ResultCard
                  key={item.path}
                  item={item}
                  checked={!!selected[item.path]}
                  onToggle={onToggle}
                />
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}
