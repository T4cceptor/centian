import type { FormEvent } from "react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";

import { defaultApiAuthHeaderName, loadStoredApiAuth, saveStoredApiAuth, clearStoredApiAuth } from "../api/api-auth";

type ApiAuthCardProps = {
  eyebrow: string;
  title: string;
  body: string;
  authHeaderName?: string;
  onSaved: () => void;
  showBackLink?: boolean;
};

export function ApiAuthCard({
  eyebrow,
  title,
  body,
  authHeaderName,
  onSaved,
  showBackLink = false,
}: ApiAuthCardProps) {
  const existingAuth = useMemo(() => loadStoredApiAuth(), []);
  const effectiveHeaderName = authHeaderName ?? existingAuth?.headerName ?? defaultApiAuthHeaderName;
  const [apiKey, setApiKey] = useState(() => existingAuth?.apiKey ?? "");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    saveStoredApiAuth({ headerName: effectiveHeaderName, apiKey });
    onSaved();
  }

  function handleClear() {
    clearStoredApiAuth();
    setApiKey("");
  }

  return (
    <div className="state-card state-card--detail" data-testid="api-auth-card">
      <p className="state-card__eyebrow">{eyebrow}</p>
      <h2>{title}</h2>
      <p>{body}</p>
      <form className="api-auth-form" onSubmit={handleSubmit}>
        <label className="api-auth-form__field">
          <span className="api-auth-form__label">Auth header</span>
          <code className="api-auth-form__hint">{effectiveHeaderName}</code>
        </label>
        <label className="api-auth-form__field" htmlFor="api-key-input">
          <span className="api-auth-form__label">API key</span>
          <input
            id="api-key-input"
            className="api-auth-form__input"
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={apiKey}
            onChange={(event) => setApiKey(event.target.value)}
            placeholder="Enter your Centian API key"
          />
        </label>
        <div className="api-auth-form__actions">
          <button className="action-button" type="submit" disabled={apiKey.trim() === ""}>
            Save and retry
          </button>
          <button className="action-button action-button--secondary" type="button" onClick={handleClear}>
            Clear stored key
          </button>
          {showBackLink ? (
            <Link className="back-link" to="/default/tasks">
              Back to task runs
            </Link>
          ) : null}
        </div>
      </form>
    </div>
  );
}
