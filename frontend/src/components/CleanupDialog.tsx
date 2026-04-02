import { formatBytes } from "../lib/format";

type Props = {
  count: number;
  totalSize: number;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => Promise<void>;
};

export default function CleanupDialog({ count, totalSize, busy, onCancel, onConfirm }: Props) {
  return (
    <div className="overlay">
      <section className="dialog">
        <h3>确认清理</h3>
        <p>
          你将清理 {count} 项内容，预计释放 {formatBytes(totalSize)}。
        </p>
        <p>建议先关闭占用目标目录的应用程序。</p>
        <div className="dialog-actions">
          <button className="ghost" onClick={onCancel} disabled={busy}>
            取消
          </button>
          <button className="primary" onClick={() => void onConfirm()} disabled={busy}>
            确认清理
          </button>
        </div>
      </section>
    </div>
  );
}
