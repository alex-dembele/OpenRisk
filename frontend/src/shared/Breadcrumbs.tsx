// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// The breadcrumb trail.
//
// What used to be here was a single flat label ("Audits") looked up from the nav
// model — no ancestors, nothing clickable. A user who reached Gap Analysis,
// Audits or Remediation therefore had no rendered way back to Compliance, which
// is exactly the reported dead end.
//
// The trail is derived from the route tree, so it cannot go stale: adding a page
// without a parent fails the routeModel test rather than shipping a dead end.
// Ancestor hrefs are filled from the matched route's own params, so the
// Framework crumb on /compliance/frameworks/abc/gaps points at
// /compliance/frameworks/abc and not at the literal pattern.

import { Link, useLocation } from 'react-router';
import { ChevronRight } from 'lucide-react';
import { routeTrail } from './routeModel';
import { useUIStrings } from './uiStrings';
import { useUIStore } from '../store/uiStore';
import { useCrumbLabels } from './crumbLabels';
import { pickLocalized } from '../i18n/locales';

export function Breadcrumbs() {
  const { pathname } = useLocation();
  const L = useUIStrings();
  const lang = useUIStore((s) => s.lang);
  const trail = routeTrail(pathname);
  // Instance names ("ISO/IEC 27001" rather than "Framework") registered by the
  // page currently rendering. Resolves after the page's own fetch, so the crumb
  // shows the generic label first and sharpens rather than flickering empty.
  const labels = useCrumbLabels();

  if (trail.length === 0) {
    return (
      <div className="flex items-center gap-2 text-[13px] min-w-0">
        <span className="text-ink-muted hidden sm:inline">{L.brandShort}</span>
      </div>
    );
  }

  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 text-[13px] min-w-0">
      <span className="text-ink-muted hidden sm:inline shrink-0">{L.brandShort}</span>
      {trail.map((step, i) => {
        const isLast = i === trail.length - 1;
        const label =
          (step.node.dynamic ? labels[step.href] : undefined) ??
          (step.node.labelKey ? L[step.node.labelKey] : pickLocalized(lang, step.node.label)) ??
          '';
        return (
          <span key={step.href} className="flex items-center gap-1.5 min-w-0">
            <ChevronRight size={13} className="text-ink-muted shrink-0 hidden sm:inline" />
            {isLast ? (
              <span aria-current="page" className="text-ink font-medium whitespace-nowrap truncate">
                {label}
              </span>
            ) : (
              // Every ancestor is a link. This is the "way back" the deep pages
              // were missing; it works with no history to pop, e.g. a deep link
              // opened in a fresh tab.
              <Link
                to={step.href}
                className="text-ink-muted hover:text-ink transition-colors whitespace-nowrap truncate hidden sm:inline"
              >
                {label}
              </Link>
            )}
          </span>
        );
      })}
    </nav>
  );
}
