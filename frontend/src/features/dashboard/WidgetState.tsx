// Copyright (c) 2026 OpenDefender Contributors
// SPDX-License-Identifier: AGPL-3.0-only
//
// What a widget renders when it does not have its data.
//
// Five conditions, five different things to say, and the whole reason this
// component exists is that five personas collapsed them into one — usually into
// the most reassuring one available:
//
//   loading         we are asking
//   error           we asked and could not read it
//   no-permission   it exists; this account may not see it
//   empty-period    there is data, just none inside the selected window
//   empty           there is nothing to show yet
//
// Before this, the exec persona rendered every failure as `?? 0` — a cyber score
// of 0 and an annual exposure of 0 FCFA, indistinguishable from a genuinely
// measured zero. The audit persona rendered a failed compliance fetch as "No
// framework — import one to start", telling the user to fix a problem they did
// not have. The viewer persona fell back to counting one page of the register
// and printed the result as a tenant total.
//
// The distinction between the last two is the one people skip, and it is the one
// that matters most on a dashboard with a period control: "no risks were opened
// in the last 7 days" and "you have no risks" are opposite facts, and only one of
// them is a reason to press a Create button.

import type { ReactNode } from 'react';
import { AlertTriangle, CalendarX, Lock, type LucideIcon } from 'lucide-react';

import { EmptyState } from '../../shared/EmptyState';
import { Btn, Skeleton } from '../../shared/ui';
import { isPermissionError } from './widgetError';
import type { LocaleCode } from '../../i18n/locales';

export interface WidgetStateProps {
  lang: LocaleCode;
  isLoading: boolean;
  error: unknown;
  /** True when the fetch succeeded and returned nothing to draw. */
  isEmpty?: boolean;
  /**
   * True when the window is what made it empty — there IS data outside it.
   * Renders the "widen the period" state instead of the "create your first"
   * state, because those have opposite remedies.
   */
  emptyBecauseOfPeriod?: boolean;
  /** Height of the loading skeleton, matched to the widget it stands in for. */
  skeletonHeight?: number;
  retry?: () => void;
  /** first-use copy: what this widget is for, and the action that fills it. */
  emptyTitle?: string;
  emptyDescription?: string;
  emptyIcon?: LucideIcon;
  emptyAction?: ReactNode;
  /** Widen the window — offered on the period-empty state. */
  onWidenPeriod?: () => void;
  children: ReactNode;
}

/**
 * Render `children` when there is data, and the right honest state otherwise.
 *
 * Note what it does NOT do: it never renders `children` with a placeholder
 * value substituted in. A widget either shows data it read or says why it
 * cannot — there is no third rendering where a zero stands in for an answer.
 */
export function WidgetState({
  lang,
  isLoading,
  error,
  isEmpty,
  emptyBecauseOfPeriod,
  skeletonHeight = 160,
  retry,
  emptyTitle,
  emptyDescription,
  emptyIcon,
  emptyAction,
  onWidenPeriod,
  children,
}: WidgetStateProps) {
  const tr = (fr: string, en: string) => (lang === 'fr' ? fr : en);

  if (isLoading) {
    return <Skeleton style={{ height: skeletonHeight }} />;
  }

  if (error) {
    if (isPermissionError(error)) {
      return (
        <EmptyState
          variant="no-permission"
          icon={Lock}
          title={tr('Indicateur non accessible', 'Metric not available to you')}
          description={tr(
            "Votre rôle ne donne pas accès à ces données. Un administrateur de l'organisation peut vous les ouvrir.",
            'Your role does not grant access to this data. An organisation administrator can grant it.',
          )}
          className="py-8"
        />
      );
    }
    return (
      <EmptyState
        variant="error"
        icon={AlertTriangle}
        title={tr('Données indisponibles', 'Data unavailable')}
        description={tr(
          "Impossible de lire cet indicateur. Aucune valeur n'est affichée tant qu'elle n'a pas été lue — réessayez, ou contactez un administrateur si cela persiste.",
          'This metric could not be read. Nothing is shown until it has been — retry, or contact an administrator if it persists.',
        )}
        primaryAction={retry ? <Btn label={tr('Réessayer', 'Retry')} onClick={retry} /> : undefined}
        className="py-8"
      />
    );
  }

  if (isEmpty && emptyBecauseOfPeriod) {
    return (
      <EmptyState
        variant="no-results"
        icon={CalendarX}
        title={tr('Rien sur cette période', 'Nothing in this period')}
        description={tr(
          'Il existe des données en dehors de la fenêtre sélectionnée. Élargissez la période pour les voir.',
          'There is data outside the selected window. Widen the period to see it.',
        )}
        primaryAction={
          onWidenPeriod ? (
            <Btn label={tr('Voir tout l’historique', 'Show all time')} onClick={onWidenPeriod} />
          ) : undefined
        }
        className="py-8"
      />
    );
  }

  if (isEmpty) {
    return (
      <EmptyState
        variant="first-use"
        icon={emptyIcon}
        title={emptyTitle ?? tr('Rien à afficher', 'Nothing to show')}
        description={emptyDescription}
        primaryAction={emptyAction}
        className="py-8"
      />
    );
  }

  return <>{children}</>;
}
