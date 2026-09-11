// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: LicenseRef-OpenRisk-Commercial
//
// IA Advisor (spec §12.3 — Assistant en langage naturel). A centered conversational
// UI wired to the LIVE backend (/ai/assistant/query): the assistant answers over the
// tenant's OWN GRC knowledge base (risks, controls, vulnerabilities) via hybrid RAG
// retrieval. Answers cite the sources they were grounded on. Claude when
// ANTHROPIC_API_KEY is set, deterministic template otherwise (badge shows which).

import { useEffect, useRef, useState } from 'react';
import { Sparkles, Send, Loader2 } from 'lucide-react';
import { useUIStore } from '../../store/uiStore';
import { useAskAssistant, useAIStatus } from './useAi';
import type { ChatTurn, KnowledgeSnippet } from './aiService';

interface AiMsg {
  role: 'ai' | 'user';
  text: string;
  sources?: string[];
  retrieved?: KnowledgeSnippet[];
}

export function AiAdvisor() {
  const lang = useUIStore((s) => s.lang);
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);
  const locale = lang === 'fr' ? 'fr' : 'en';

  const status = useAIStatus();
  const ask = useAskAssistant();

  const [msgs, setMsgs] = useState<AiMsg[]>([
    {
      role: 'ai',
      text: tr(
        'Bonjour. Je suis votre assistant GRC OpenRisk. Je réponds à vos questions à partir de VOTRE base de connaissances (risques, contrôles de conformité, vulnérabilités). Posez-moi une question — par exemple sur vos risques critiques, votre conformité ISO 27001, ou une CVE.',
        'Hello. I am your OpenRisk GRC assistant. I answer from YOUR knowledge base (risks, compliance controls, vulnerabilities). Ask me a question — e.g. about your critical risks, ISO 27001 compliance, or a CVE.',
      ),
    },
  ]);
  const [input, setInput] = useState('');
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [msgs, ask.isPending]);

  const send = (q?: string) => {
    const t = (q ?? input).trim();
    if (!t || ask.isPending) return;

    // Build the conversation history from the existing turns (excludes the greeting).
    const history: ChatTurn[] = msgs.slice(1).map((m) => ({
      role: m.role === 'ai' ? ('assistant' as const) : ('user' as const),
      text: m.text,
    }));

    setMsgs((m) => [...m, { role: 'user', text: t }]);
    setInput('');

    ask.mutate(
      { question: t, history, locale },
      {
        onSuccess: (res) =>
          setMsgs((m) => [
            ...m,
            {
              role: 'ai',
              text: res.answer.answer,
              sources: res.answer.sources,
              retrieved: res.retrieved,
            },
          ]),
        onError: () =>
          setMsgs((m) => [
            ...m,
            {
              role: 'ai',
              text: tr(
                "Désolé, je n'ai pas pu traiter votre demande. Réessayez dans un instant.",
                'Sorry, I could not process your request. Please try again shortly.',
              ),
            },
          ]),
      },
    );
  };

  const sugg = [
    tr('Quels sont mes risques les plus critiques ?', 'What are my most critical risks?'),
    tr('Résume ma conformité ISO 27001', 'Summarize my ISO 27001 compliance'),
    tr('Sommes-nous exposés à Log4Shell ?', 'Are we exposed to Log4Shell?'),
  ];

  const llmOn = status.data?.llm_enabled;
  const model = status.data?.model;

  return (
    <div className="flex flex-col" style={{ height: 'calc(100vh - 58px)' }}>
      {/* The page had no heading at all — no h1, no h2, nothing. Every other
          screen in the app opens with one, and without it a screen-reader user
          landing here has no way to tell which page they are on, nor to jump to
          it from the headings list. It is visually hidden because the screen's
          own chrome (the provenance badge, the conversation) already tells a
          sighted user where they are. */}
      <h1 className="sr-only">{tr('IA Advisor', 'AI Advisor')}</h1>

      {/* Provenance badge */}
      <div
        className="shrink-0 flex items-center justify-center gap-2 py-2.5"
        style={{ borderBottom: '1px solid var(--border)' }}
      >
        <span
          className="text-[11px] font-semibold px-2.5 py-1 rounded-full"
          style={{
            background: llmOn ? 'var(--accent-soft)' : 'var(--bg-elevated)',
            color: llmOn ? 'var(--accent)' : 'var(--ink-muted)',
            border: `1px solid ${llmOn ? 'var(--accent-line)' : 'var(--border-strong)'}`,
          }}
        >
          {status.isLoading
            ? tr('Vérification…', 'Checking…')
            : llmOn
              ? tr(`IA Claude active (${model})`, `Claude AI active (${model})`)
              : tr('Mode local (sans clé API)', 'Local mode (no API key)')}
        </span>
      </div>

      <div ref={scrollRef} className="flex-1 overflow-y-auto py-7">
        <div className="max-w-[740px] mx-auto px-6 flex flex-col gap-5">
          {msgs.map((x, i) =>
            x.role === 'ai' ? (
              <div key={i} className="flex gap-3.5">
                <div
                  className="w-[34px] h-[34px] rounded-[10px] flex items-center justify-center text-accent-strong shrink-0"
                  style={{ background: 'var(--accent-soft)' }}
                >
                  <Sparkles size={18} />
                </div>
                <div className="min-w-0">
                  <div
                    className="text-[14px] leading-relaxed text-ink pt-1"
                    style={{ whiteSpace: 'pre-line' }}
                  >
                    {x.text}
                  </div>
                  {x.sources && x.sources.length > 0 && (
                    <div className="flex flex-wrap gap-1.5 mt-2.5">
                      {x.sources.map((s, si) => (
                        <span
                          key={si}
                          className="text-[11px] px-2 py-0.5 rounded-md text-ink-soft"
                          style={{
                            background: 'var(--bg-elevated)',
                            border: '1px solid var(--border)',
                          }}
                          title={tr('Source utilisée', 'Source used')}
                        >
                          {s}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            ) : (
              <div
                key={i}
                className="self-end max-w-[80%] text-[14px] leading-relaxed text-ink px-[15px] py-[11px] rounded-[15px]"
                style={{ background: 'var(--accent-soft)', border: '1px solid var(--accent-line)' }}
              >
                {x.text}
              </div>
            ),
          )}
          {ask.isPending && (
            <div className="flex gap-3.5 items-center">
              <div
                className="w-[34px] h-[34px] rounded-[10px] flex items-center justify-center text-accent-strong shrink-0"
                style={{ background: 'var(--accent-soft)' }}
              >
                <Sparkles size={18} />
              </div>
              <div className="flex items-center gap-2 text-[13px] text-ink-muted pt-1">
                <Loader2 size={15} className="animate-spin" />
                {tr('Analyse de votre base GRC…', 'Analysing your GRC data…')}
              </div>
            </div>
          )}
        </div>
      </div>

      <div className="shrink-0 py-4" style={{ borderTop: '1px solid var(--border)' }}>
        <div className="max-w-[740px] mx-auto px-6">
          {msgs.length <= 1 && (
            <div className="flex gap-2 mb-3 flex-wrap">
              {sugg.map((sq) => (
                <button
                  key={sq}
                  onClick={() => send(sq)}
                  className="text-[12.5px] font-medium px-3.5 py-2 rounded-full text-ink-soft hover:text-accent-strong transition-colors"
                  style={{
                    border: '1px solid var(--border-strong)',
                    background: 'var(--bg-elevated)',
                  }}
                >
                  {sq}
                </button>
              ))}
            </div>
          )}
          <div className="flex gap-2.5 items-center">
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && send()}
              disabled={ask.isPending}
              placeholder={tr(
                'Posez une question à votre assistant GRC…',
                'Ask your GRC assistant…',
              )}
              className="flex-1 h-12 px-[18px] rounded-[14px] text-[14px] text-ink outline-none disabled:opacity-60"
              style={{ border: '1px solid var(--border-strong)', background: 'var(--bg-elevated)' }}
            />
            {/* An icon-only control needs a name, or it is announced as
                "button" and nothing more — the one control that sends the
                message. The label follows the state so a screen reader is told
                the send is in flight rather than silently ignored. */}
            <button
              onClick={() => send()}
              disabled={ask.isPending}
              aria-label={
                ask.isPending ? tr('Envoi en cours…', 'Sending…') : tr('Envoyer', 'Send')
              }
              title={ask.isPending ? tr('Envoi en cours…', 'Sending…') : tr('Envoyer', 'Send')}
              className="w-12 h-12 rounded-[14px] flex items-center justify-center text-fg-primary shrink-0 disabled:opacity-60"
              style={{ background: 'var(--accent-solid)', color: 'var(--fg-on-solid)' }}
            >
              {ask.isPending ? (
                <Loader2 size={20} className="animate-spin" aria-hidden="true" />
              ) : (
                <Send size={20} aria-hidden="true" />
              )}
            </button>
          </div>
          <div className="text-center text-[11px] text-ink-muted mt-2.5">
            {tr(
              "L'IA répond à partir de vos données GRC et peut se tromper. Vérifiez les décisions critiques.",
              'The AI answers from your GRC data and can make mistakes. Verify critical decisions.',
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
