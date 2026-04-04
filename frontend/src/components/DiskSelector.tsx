import { useState } from "react";

type Props = {
  onStart: (drive: string) => Promise<void>;
  busy: boolean;
};

export default function DiskSelector({ onStart, busy }: Props) {
  const [drive, setDrive] = useState("D:");

  return (
    <section className="disk-selector">
      <h2>选择扫描磁盘</h2>
      <div className="grid-two">
        <label>
          驱动器
          <select value={drive} onChange={(e) => setDrive(e.target.value)} disabled={busy}>
            <option value="C:">C:</option>
            <option value="D:">D:</option>
            <option value="E:">E:</option>
            <option value="F:">F:</option>
          </select>
        </label>
      </div>
      <p>系统目录会自动跳过，扫描策略为自适应深度 + TopN 压缩。</p>
      <p>提示：扫描 C: 为获取更完整结果，建议使用管理员权限启动。</p>
      <button className="primary" onClick={() => void onStart(drive)} disabled={busy}>
        开始智能扫描
      </button>
    </section>
  );
}
