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
  const timeoutValid = draft.llm.request_timeout_seconds > 0;

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
          请求超时（秒）
          <input
            type="number"
            min={1}
            value={draft.llm.request_timeout_seconds}
            onChange={(e) => {
              const parsed = Number(e.target.value);
              const next = Number.isFinite(parsed) ? Math.trunc(parsed) : 120;
              setDraft({ ...draft, llm: { ...draft.llm, request_timeout_seconds: next } });
            }}
          />
          <small>单次 LLM 请求超时时间，建议 60-300 秒。</small>
          {!timeoutValid && <small>请输入大于 0 的整数。</small>}
        </label>
        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={draft.llm.enable_streaming}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, enable_streaming: e.target.checked } })}
          />
          <span>启用流式输出（开 / 关）</span>
        </label>
        <small>开启后会使用流式返回，可减少首字等待，但取决于模型服务端是否支持。</small>
        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={draft.llm.enable_web_search}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, enable_web_search: e.target.checked } })}
          />
          <span>启用网络搜索工具（Tavily）</span>
        </label>
        <small>关闭时模型不会调用网络搜索工具；开启后需配置 Tavily API Key 才会生效。</small>
        <label>
          Tavily API Key
          <input
            type="password"
            value={draft.llm.tavily_api_key}
            disabled={!draft.llm.enable_web_search}
            onChange={(e) => setDraft({ ...draft, llm: { ...draft.llm, tavily_api_key: e.target.value } })}
          />
        </label>
        <label>
          Tavily Base URL
          <input
            value={draft.llm.tavily_base_url}
            disabled={!draft.llm.enable_web_search}
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
            disabled={!maxTokensValid || !maxTurnsValid || !timeoutValid}
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
