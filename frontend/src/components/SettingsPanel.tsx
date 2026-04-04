import { useState } from "react";
import type { AppConfig } from "../lib/types";

type Props = {
  config: AppConfig;
  onSave: (cfg: AppConfig) => Promise<void>;
  onClose: () => void;
  saveError?: string;
};

export default function SettingsPanel({ config, onSave, onClose, saveError }: Props) {
  const [draft, setDraft] = useState(config);
  const maxTokensValid = draft.llm.max_tokens === -1 || draft.llm.max_tokens > 0;
  const maxTurnsValid = draft.llm.max_turns === -1 || draft.llm.max_turns > 0;

  return (
    <div className="overlay">
      <section className="settings">
        <h3>LLM 设置</h3>
        <label>
          Provider
          <input
            value={draft.llm.provider}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, provider: e.target.value } })}
          />
        </label>
        <label>
          API Key
          <input
            type="password"
            value={draft.llm.api_key}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, api_key: e.target.value } })}
          />
        </label>
        <label>
          Model
          <input
            value={draft.llm.model}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, model: e.target.value } })}
          />
        </label>
        <label>
          Base URL
          <input
            value={draft.llm.base_url}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, base_url: e.target.value } })}
          />
        </label>
        <label>
          Tavily API Key
          <input
            type="password"
            value={draft.llm.tavily_api_key}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, tavily_api_key: e.target.value } })}
          />
        </label>
        <label>
          Tavily Base URL
          <input
            value={draft.llm.tavily_base_url}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, tavily_base_url: e.target.value } })}
          />
          <small>默认 https://api.tavily.com</small>
        </label>
        <label>
          Max Tokens
          <input
            type="number"
            min={-1}
            value={draft.llm.max_tokens}
            onChange={(e) => {
              const parsed = Number(e.target.value);
              const next = Number.isFinite(parsed) ? Math.trunc(parsed) : 1200;
              setDraft({ ...draft, llm: { ...draft.llm, max_tokens: next } });
            }}
          />
          <small>-1 表示不限制（由模型服务端按默认上限处理）。</small>
          {!maxTokensValid && <small>请输入 -1 或正整数。</small>}
        </label>
        <label>
          Max Turns
          <input
            type="number"
            min={-1}
            value={draft.llm.max_turns}
            onChange={(e) => {
              const parsed = Number(e.target.value);
              const next = Number.isFinite(parsed) ? Math.trunc(parsed) : 6;
              setDraft({ ...draft, llm: { ...draft.llm, max_turns: next } });
            }}
          />
          <small>-1 表示不限制（直到模型返回结构化结果或请求被取消）。</small>
          {!maxTurnsValid && <small>请输入 -1 或正整数。</small>}
        </label>
        <div className="dialog-actions">
          <button className="ghost" onClick={onClose}>
            关闭
          </button>
          <button
            className="primary"
            disabled={!maxTokensValid || !maxTurnsValid}
            onClick={() => {
              void onSave(draft);
            }}
          >
            保存
          </button>
        </div>
        {saveError && <p className="error-text">{saveError}</p>}
      </section>
    </div>
  );
}
