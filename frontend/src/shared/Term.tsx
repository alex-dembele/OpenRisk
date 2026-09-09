// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// Inline glossary term (docs/UX_CHARTER UX-14, OR-BUG-010): renders an acronym as
// a semantic <abbr> with a dotted underline and a first-hover plain-language
// definition, in the current locale. Falls back to plain text for unknown terms.

import type { ReactNode } from 'react';
import { GLOSSARY } from './glossary';
import { useUIStore } from '../store/uiStore';
import { pickLocalized } from '../i18n/locales';

export function Term({ term, children }: { term: string; children?: ReactNode }) {
  const lang = useUIStore((s) => s.lang);
  const def = pickLocalized(lang, GLOSSARY[term]);
  const content = children ?? term;
  if (!def) return <>{content}</>;
  return (
    <abbr
      title={def}
      style={{ textDecoration: 'underline dotted', textUnderlineOffset: 3, cursor: 'help' }}
    >
      {content}
    </abbr>
  );
}
