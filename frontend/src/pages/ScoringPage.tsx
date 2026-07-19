import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, ProgressBar,
  Label, CounterLabel,
} from '@primer/react';
import { keyBy, mapValues, every, debounce } from 'lodash';
import type { Session, Patrol, Submission, DraftComment, WSDraftUpdatedPayload, WSPresenceUpdatedPayload, WSPresenceStatePayload, WSCommentUpdatedPayload, WSSessionFinalisedPayload, WSSessionLockedPayload, WSSessionUnlockedPayload, WSServerMessage } from '../lib/types';
import * as api from '../lib/api';
import { useDraftSync, useSessionSubscription, usePresence } from '../hooks/useWebSocket';
import { useAuth } from '../hooks/useAuth';
import { ScoreSlider } from '../components/ScoreSlider';

type View = 'scoring' | 'summary' | 'viewing';

export const ScoringPage = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();

  const [session, setSession] = useState<Session | null>(null);
  const [patrols, setPatrols] = useState<Patrol[]>([]);
  const [submissions, setSubmissions] = useState<Submission[]>([]);
  const [currentPatrolIndex, setCurrentPatrolIndex] = useState(0);
  const [scores, setScores] = useState<Record<string, number | null>>({});
  const [comments, setComments] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [unlockingSession, setUnlockingSession] = useState(false);
  const [error, setError] = useState('');
  const [view, setView] = useState<View>('scoring');
  const [jumpedFromSummary, setJumpedFromSummary] = useState(false);
  const [viewingScores, setViewingScores] = useState<Record<string, number>>({});
  const [revising, setRevising] = useState(false);
  // Per-user comments from the server (patrol_id → DraftComment[])
  const [perUserComments, setPerUserComments] = useState<Record<string, DraftComment[]>>({});

  // Per-user submitted comments for read-only viewing mode
  const [viewingPerUserComments, setViewingPerUserComments] = useState<DraftComment[]>([]);

  // Which criterion the current user is commenting on (for presence broadcasting)
  const [commentingOn, setCommentingOn] = useState<string | undefined>(undefined);

  // Award state
  const [awards, setAwards] = useState<Record<string, string>>({}); // award_type → patrol_id
  const [previousTotals, setPreviousTotals] = useState<Record<string, number>>({}); // patrol_id → previous total
  const [previousTotalsLoaded, setPreviousTotalsLoaded] = useState(false);

  // Track total score per patrol (patrol_id → total)
  const [patrolTotals, setPatrolTotals] = useState<Record<string, number | null>>({});

  // Lock screen state for either subcamp finalisation or full session lock.
  const [lockState, setLockState] = useState<{ kind: 'subcamp' | 'session'; displayName: string; actionAt: string; endsAt: string } | null>(null);

  // Track which patrols have had all criteria touched (keyed by patrol_id)
  const [touchedMap, setTouchedMap] = useState<Record<string, Set<string>>>({});

  // Live multiplayer: track who is present on each patrol and whether they're editing
  // patrol_id → user_id → { user_name, at, mode, commenting_on? }
  const [presenceMap, setPresenceMap] = useState<Record<string, Record<string, { user_name: string; at: number; mode: 'viewing' | 'editing'; commenting_on?: string }>>>({});
  // Periodic re-render to expire stale presence entries (every 10s)
  const [, setPresenceTick] = useState(0);
  useEffect(() => {
    const timer = setInterval(() => setPresenceTick((t) => t + 1), 10_000);
    return () => clearInterval(timer);
  }, []);

  // Ref for current patrol ID to use in WS callback
  const currentPatrolIdRef = useRef<string>('');

  const currentPatrol = patrols[currentPatrolIndex];
  const criteria = session?.criteria ?? [];
  const patrolLabel = (patrol: Patrol): string => {
    if (session?.round_type === 'round2') {
      return patrol.subcamp || patrol.name;
    }
    return patrol.name;
  };
  const patrolTitle = (patrol: Patrol): string => {
    if (session?.round_type === 'round2' && patrol.subcamp) {
      return `${patrol.subcamp} - ${patrol.name}`;
    }
    return patrol.name;
  };

  // Keep ref in sync
  useEffect(() => {
    currentPatrolIdRef.current = currentPatrol?.patrol_id ?? '';
  }, [currentPatrol?.patrol_id]);

  // Draft sync over WebSocket
  const { saveDraft, flushDraft } = useDraftSync(
    sessionId ?? '',
    currentPatrol?.patrol_id ?? '',
  );

  // Per-criterion debounced comment savers (REST API)
  const commentSaversRef = useRef<Record<string, ReturnType<typeof debounce>>>({});
  const getCommentSaver = useCallback((criterionId: string) => {
    if (!commentSaversRef.current[criterionId]) {
      commentSaversRef.current[criterionId] = debounce(
        (sid: string, pid: string, comment: string) => {
          api.saveDraftComment(sid, pid, criterionId, comment).catch((err) => {
            console.error('[comment] Save failed:', err);
          });
        },
        800,
      );
    }
    return commentSaversRef.current[criterionId];
  }, []);

  // Flush all pending debounced comment saves immediately
  const flushAllCommentSaves = useCallback(() => {
    for (const saver of Object.values(commentSaversRef.current)) {
      saver.flush();
    }
  }, []);

  // Flush pending comment saves on unmount and before page reload
  useEffect(() => {
    const handleBeforeUnload = () => {
      flushAllCommentSaves();
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
      flushAllCommentSaves();
    };
  }, [flushAllCommentSaves]);

  // Subscribe to session updates — handles live multiplayer draft updates + presence
  const handleWSMessage = useCallback((msg: WSServerMessage) => {
    if (msg.type === 'presence_state') {
      const payload = msg.payload as WSPresenceStatePayload;
      setPresenceMap((prev) => {
        const next = { ...prev };
        for (const entry of payload.users) {
          const patrolUsers = { ...(next[entry.patrol_id] ?? {}) };
          const existing = patrolUsers[entry.user_id];
          // Don't overwrite a recent 'editing' state
          if (existing?.mode === 'editing' && (Date.now() - existing.at) < 30000) continue;
          patrolUsers[entry.user_id] = { user_name: entry.user_name, at: Date.now(), mode: 'viewing', commenting_on: entry.commenting_on };
          next[entry.patrol_id] = patrolUsers;
        }
        return next;
      });
    }

    if (msg.type === 'presence_updated') {
      const payload = msg.payload as WSPresenceUpdatedPayload;
      const patrolId = payload.patrol_id;
      setPresenceMap((prev) => {
        const patrolUsers = { ...(prev[patrolId] ?? {}) };
        const existing = patrolUsers[payload.user_id];
        // Don't downgrade from 'editing' to 'viewing' if the editing timestamp is recent (< 30s)
        if (existing?.mode === 'editing' && (Date.now() - existing.at) < 30000) {
          patrolUsers[payload.user_id] = { ...existing, at: Date.now(), commenting_on: payload.commenting_on };
        } else {
          patrolUsers[payload.user_id] = { user_name: payload.user_name, at: Date.now(), mode: 'viewing', commenting_on: payload.commenting_on };
        }
        return { ...prev, [patrolId]: patrolUsers };
      });
    }

    if (msg.type === 'comment_updated') {
      const payload = msg.payload as WSCommentUpdatedPayload;
      const patrolId = payload.patrol_id;
      setPerUserComments((prev) => {
        const existing = [...(prev[patrolId] ?? [])];
        if (payload.comment === '') {
          // Delete
          return { ...prev, [patrolId]: existing.filter(
            (c) => !(c.criterion_id === payload.criterion_id && c.user_id === payload.user_id)
          )};
        }
        // Upsert
        const idx = existing.findIndex(
          (c) => c.criterion_id === payload.criterion_id && c.user_id === payload.user_id
        );
        const entry: DraftComment = {
          criterion_id: payload.criterion_id,
          user_id: payload.user_id,
          display_name: payload.display_name,
          comment: payload.comment,
          updated_at: new Date().toISOString(),
        };
        if (idx >= 0) {
          existing[idx] = entry;
        } else {
          existing.push(entry);
        }
        return { ...prev, [patrolId]: existing };
      });
    }

    if (msg.type === 'draft_updated') {
      const payload = msg.payload as WSDraftUpdatedPayload;
      // The server already excludes the sending WebSocket connection from
      // the broadcast, so we don't filter by user_id here — that would
      // break same-user multi-tab sync.

      const patrolId = payload.patrol_id;

      // Upgrade presence to 'editing'
      setPresenceMap((prev) => {
        const patrolUsers = { ...(prev[patrolId] ?? {}) };
        patrolUsers[payload.user_id] = { user_name: payload.user_name, at: Date.now(), mode: 'editing' };
        return { ...prev, [patrolId]: patrolUsers };
      });

      // Update touched map + totals for this patrol
      setTouchedMap((prev) => {
        const existing = prev[patrolId] ?? new Set();
        const updated = new Set(existing);
        for (const cid of Object.keys(payload.scores)) {
          updated.add(cid);
        }
        return { ...prev, [patrolId]: updated };
      });

      // Update patrol total
      let total = 0;
      for (const v of Object.values(payload.scores)) {
        total += v;
      }
      setPatrolTotals((prev) => ({ ...prev, [patrolId]: total }));

      // If this update is for the patrol we're currently viewing, merge into local state
      if (patrolId === currentPatrolIdRef.current) {
        setScores((prev) => {
          const next = { ...prev };
          for (const [cid, value] of Object.entries(payload.scores)) {
            next[cid] = value;
          }
          return next;
        });
      }
    }

    // Handle session finalised by another user — show lock screen
    if (msg.type === 'session_finalised') {
      const payload = msg.payload as WSSessionFinalisedPayload;

      // Only lock this scorer when their own subcamp is finalised.
      // Users without a subcamp assignment should not enter the scorer lock state.
      if (!user?.subcamp_id || payload.subcamp_id !== user.subcamp_id) {
        return;
      }

      setLockState({
        kind: 'subcamp',
        displayName: payload.user_display_name,
        actionAt: payload.finalised_at,
        endsAt: payload.ends_at,
      });
    }

    if (msg.type === 'session_locked') {
      const payload = msg.payload as WSSessionLockedPayload;
      setLockState({
        kind: 'session',
        displayName: payload.user_display_name,
        actionAt: payload.locked_at,
        endsAt: payload.ends_at,
      });
    }

    if (msg.type === 'session_unlocked') {
      const payload = msg.payload as WSSessionUnlockedPayload;
      if (payload.session_id === sessionId) {
        setLockState((prev) => (prev?.kind === 'session' ? null : prev));
      }
    }
  }, [user?.subcamp_id, sessionId]);

  useSessionSubscription(sessionId, handleWSMessage);

  // Send presence heartbeats for the current patrol (includes commentingOn)
  usePresence(sessionId, currentPatrol?.patrol_id, commentingOn);

  // Load session data
  useEffect(() => {
    if (!sessionId) return;

    api.getSession(sessionId)
      .then(({ session, patrols, submissions, awards: savedAwards }) => {
        setSession(session);

        if (session.status === 'LOCKED') {
          setLockState({
            kind: 'session',
            displayName: session.locked_by_name || 'an administrator',
            actionAt: session.locked_at || new Date().toISOString(),
            endsAt: session.ends_at,
          });
        }

        setPatrols(patrols);
        setSubmissions(submissions);

        // Restore saved award selections
        if (savedAwards?.length) {
          const awardMap: Record<string, string> = {};
          for (const a of savedAwards) {
            awardMap[a.award_type] = a.patrol_id;
          }
          setAwards(awardMap);
        }

        // Load previous session totals if most_improved is enabled
        if (session.award_most_improved && session.previous_session_id) {
          api.getPreviousScores(sessionId).then(({ totals }) => {
            const map: Record<string, number> = {};
            for (const t of totals) {
              map[t.patrol_id] = t.total;
            }
            setPreviousTotals(map);
            setPreviousTotalsLoaded(true);
          }).catch(() => setPreviousTotalsLoaded(true));
        } else {
          setPreviousTotalsLoaded(true);
        }

        // If all patrols already submitted, go to summary
        const submittedIds = new Set(submissions.map((s) => s.patrol_id));
        const allDone = patrols.length > 0 && every(patrols, (p) => submittedIds.has(p.patrol_id));
        if (allDone) {
          setView('summary');
          // Load totals for submitted patrols
          for (const patrol of patrols) {
            api.getSubmissionScores(sessionId, patrol.patrol_id).then(({ scores }) => {
              const total = scores.reduce((sum, s) => sum + s.value, 0);
              setPatrolTotals((prev) => ({ ...prev, [patrol.patrol_id]: total }));
            }).catch(() => { /* ignore */ });
          }
        } else {
          // Start at first unsubmitted patrol
          const firstUnsubmitted = patrols.findIndex(
            (p) => !submittedIds.has(p.patrol_id),
          );
          if (firstUnsubmitted >= 0) {
            setCurrentPatrolIndex(firstUnsubmitted);
          }

          // Pre-load draft status for ALL non-submitted patrols so
          // touchedMap + patrolTotals are correct if the user jumps to summary
          for (const patrol of patrols) {
            if (submittedIds.has(patrol.patrol_id)) continue;
            api.getDraft(sessionId, patrol.patrol_id).then(({ draft }) => {
              if (draft?.scores?.length) {
                setTouchedMap((prev) => ({
                  ...prev,
                  [patrol.patrol_id]: new Set(draft.scores.map((s) => s.criterion_id)),
                }));
                const total = draft.scores.reduce((sum, s) => sum + s.value, 0);
                setPatrolTotals((prev) => ({ ...prev, [patrol.patrol_id]: total }));
              }
            }).catch(() => { /* ignore */ });
            // Load per-user comments for this patrol
            api.getDraftComments(sessionId, patrol.patrol_id).then(({ comments: cmts }) => {
              if (cmts?.length) {
                setPerUserComments((prev) => ({
                  ...prev,
                  [patrol.patrol_id]: cmts.map((c) => ({
                    criterion_id: c.criterion_id,
                    user_id: c.user_id,
                    display_name: c.display_name,
                    comment: c.comment,
                    updated_at: c.updated_at,
                  })),
                }));
              }
            }).catch(() => { /* ignore */ });
          }
        }
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [sessionId]);

  // Load draft when patrol changes
  useEffect(() => {
    if (!sessionId || !currentPatrol || view !== 'scoring') return;

    // Load draft scores
    api.getDraft(sessionId, currentPatrol.patrol_id).then(({ draft }) => {
      if (draft?.scores?.length) {
        const restored: Record<string, number | null> = mapValues(
          keyBy(draft.scores, 'criterion_id'),
          'value',
        );
        setScores(restored);

        // Mark all restored criteria as touched
        setTouchedMap((prev) => ({
          ...prev,
          [currentPatrol.patrol_id]: new Set(draft.scores.map((s) => s.criterion_id)),
        }));

        // Update patrol total
        const total = draft.scores.reduce((sum, s) => sum + s.value, 0);
        setPatrolTotals((prev) => ({ ...prev, [currentPatrol.patrol_id]: total }));
      } else {
        // Initialize with null — slider shows at 0 but dimmed/unset
        const initial: Record<string, number | null> = {};
        for (const c of criteria) {
          initial[c.id] = null;
        }
        setScores(initial);
      }
    });

    // Load per-user comments (all users) and extract own comments for the textarea
    api.getDraftComments(sessionId, currentPatrol.patrol_id).then(({ comments: cmts }) => {
      if (cmts?.length) {
        const mapped: DraftComment[] = cmts.map((c) => ({
          criterion_id: c.criterion_id,
          user_id: c.user_id,
          display_name: c.display_name,
          comment: c.comment,
          updated_at: c.updated_at,
        }));
        setPerUserComments((prev) => ({ ...prev, [currentPatrol.patrol_id]: mapped }));

        // Extract the current user's comments for the local textarea state
        const ownComments: Record<string, string> = {};
        for (const c of criteria) {
          ownComments[c.id] = '';
        }
        for (const c of mapped) {
          if (c.user_id === user?.id) {
            ownComments[c.criterion_id] = c.comment;
          }
        }
        setComments(ownComments);
      } else {
        // No comments yet — initialize empty
        const initialComments: Record<string, string> = {};
        for (const c of criteria) {
          initialComments[c.id] = '';
        }
        setComments(initialComments);
      }
    }).catch(() => {
      // Fallback: initialize empty comments
      const initialComments: Record<string, string> = {};
      for (const c of criteria) {
        initialComments[c.id] = '';
      }
      setComments(initialComments);
    });
  }, [sessionId, currentPatrol?.patrol_id, criteria, view, user?.id]);

  // Auto-save scores when they change (only save non-null values)
  const handleScoreChange = useCallback(
    (criterionId: string, value: number) => {
      setScores((prev) => {
        const next = { ...prev, [criterionId]: value };

        // Build only non-null scores for WebSocket save
        const saveable: Record<string, number> = {};
        let total = 0;
        for (const [k, v] of Object.entries(next)) {
          if (v !== null) {
            saveable[k] = v;
            total += v;
          }
        }
        if (Object.keys(saveable).length > 0) {
          saveDraft(saveable);
        }

        // Update patrol total
        if (currentPatrol) {
          setPatrolTotals((prev) => ({ ...prev, [currentPatrol.patrol_id]: total }));
        }

        return next;
      });

      // Track this criterion as touched for the current patrol
      if (currentPatrol) {
        setTouchedMap((prev) => {
          const existing = prev[currentPatrol.patrol_id] ?? new Set();
          const updated = new Set(existing);
          updated.add(criterionId);
          return { ...prev, [currentPatrol.patrol_id]: updated };
        });
      }
    },
    [saveDraft, currentPatrol],
  );

  // Handle comment change — save via REST API (debounced per criterion)
  const handleCommentChange = useCallback(
    (criterionId: string, newComment: string) => {
      setComments((prev) => ({ ...prev, [criterionId]: newComment }));

      // Debounced REST save
      if (sessionId && currentPatrol) {
        getCommentSaver(criterionId)(sessionId, currentPatrol.patrol_id, newComment);
      }
    },
    [sessionId, currentPatrol?.patrol_id, getCommentSaver],
  );

  // Delete the current user's comment on a criterion
  const handleCommentDelete = useCallback(
    async (criterionId: string) => {
      if (!sessionId || !currentPatrol) return;
      setComments((prev) => ({ ...prev, [criterionId]: '' }));
      try {
        await api.deleteDraftComment(sessionId, currentPatrol.patrol_id, criterionId);
      } catch (err) {
        console.error('[comment] Delete failed:', err);
      }
    },
    [sessionId, currentPatrol?.patrol_id],
  );

  // Track which criterion the user is commenting on (for presence broadcasting)
  const handleCommentFocus = useCallback(
    (criterionId: string) => {
      setCommentingOn(criterionId);
    },
    [],
  );

  const handleCommentBlur = useCallback(() => {
    setCommentingOn(undefined);
  }, []);

  // Check if all criteria for a patrol are touched
  const isPatrolComplete = useCallback(
    (patrolId: string) => {
      const touched = touchedMap[patrolId];
      if (!touched || criteria.length === 0) return false;
      return criteria.every((c) => touched.has(c.id));
    },
    [touchedMap, criteria],
  );

  // Navigate between patrols
  const goToPatrol = useCallback(
    async (index: number) => {
      flushAllCommentSaves();
      await flushDraft();
      setCurrentPatrolIndex(index);
      setView('scoring');
      setJumpedFromSummary(false);
    },
    [flushDraft, flushAllCommentSaves],
  );

  const goToSummary = useCallback(async () => {
    flushAllCommentSaves();
    await flushDraft();
    setView('summary');
    setJumpedFromSummary(false);
  }, [flushDraft, flushAllCommentSaves]);

  // Jump from summary to a specific patrol, with "Back to Summary" nav
  const jumpToPatrolFromSummary = useCallback(
    async (index: number) => {
      flushAllCommentSaves();
      await flushDraft();
      setCurrentPatrolIndex(index);
      setView('scoring');
      setJumpedFromSummary(true);
    },
    [flushDraft, flushAllCommentSaves],
  );

  const goNext = useCallback(() => {
    if (currentPatrolIndex < patrols.length - 1) {
      goToPatrol(currentPatrolIndex + 1);
    } else {
      // Last patrol — go to summary
      goToSummary();
    }
  }, [currentPatrolIndex, patrols.length, goToPatrol, goToSummary]);

  const goPrev = useCallback(() => {
    if (currentPatrolIndex > 0) {
      goToPatrol(currentPatrolIndex - 1);
    }
  }, [currentPatrolIndex, goToPatrol]);

  // Finalise — submit all patrols at once
  const handleFinalise = useCallback(async () => {
    if (!sessionId) return;
    setShowConfirmFinalise(false);
    setSubmitting(true);
    setError('');

    try {
      flushAllCommentSaves();
      await flushDraft();
      const result = await api.finaliseSession(sessionId);
      setSubmissions(result.submissions);

      // Navigate to dashboard with success feedback
      navigate('/', { state: { finalised: session?.name ?? 'Session' } });
    } catch (err) {
      console.error('[finalise] Error:', err);
      setError(err instanceof Error ? err.message : 'Finalise failed');
      setSubmitting(false);
    }
  }, [sessionId, session, flushDraft, flushAllCommentSaves, navigate]);

  // View submitted scores for a patrol (read-only)
  const viewPatrolScores = useCallback(
    async (patrolIndex: number) => {
      if (!sessionId) return;
      const patrol = patrols[patrolIndex];
      if (!patrol) return;

      try {
        const { scores: submissionScores } = await api.getSubmissionScores(
          sessionId,
          patrol.patrol_id,
        );
        const scoreMap: Record<string, number> = {};
        for (const s of submissionScores) {
          scoreMap[s.criterion_id] = s.value;
        }
        setViewingScores(scoreMap);

        // Fetch per-user submitted comments
        try {
          const { comments: submittedComments } = await api.getSubmittedComments(sessionId, patrol.patrol_id);
          setViewingPerUserComments(
            (submittedComments ?? []).map((c) => ({
              criterion_id: c.criterion_id,
              user_id: c.user_id,
              display_name: c.display_name,
              comment: c.comment,
              updated_at: c.updated_at,
            })),
          );
        } catch {
          setViewingPerUserComments([]);
        }

        setCurrentPatrolIndex(patrolIndex);
        setView('viewing');
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Could not load scores');
      }
    },
    [sessionId, patrols],
  );

  // Revise — convert submissions back to drafts for editing
  const handleRevise = useCallback(async () => {
    if (!sessionId) return;
    setRevising(true);
    setError('');

    try {
      await api.reviseSession(sessionId);
      // Clear submissions, awards, and put user back in scoring mode
      setSubmissions([]);
      setAwards({});
      setTouchedMap({});
      setPatrolTotals({});
      setCurrentPatrolIndex(0);
      setView('scoring');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not revise scores');
    } finally {
      setRevising(false);
    }
  }, [sessionId]);

  // ─── Award logic ───────────────────────────────────────────────

  const hasAwards = session?.award_best_patrol || session?.award_most_improved;
  const hasPreviousSession = !!session?.previous_session_id;

  // Auto-calculate best patrol (highest total)
  const suggestedBestPatrol = useMemo(() => {
    if (!patrols.length) return null;
    let best: { id: string; total: number } | null = null;
    for (const p of patrols) {
      const total = patrolTotals[p.patrol_id];
      if (total != null && (best === null || total > best.total)) {
        best = { id: p.patrol_id, total };
      }
    }
    return best?.id ?? null;
  }, [patrols, patrolTotals]);

  // Auto-calculate most improved (biggest net improvement from previous session)
  const suggestedMostImproved = useMemo(() => {
    if (!patrols.length || !previousTotalsLoaded || !hasPreviousSession) return null;
    // Need at least one previous total to compare
    if (Object.keys(previousTotals).length === 0) return null;

    let best: { id: string; delta: number } | null = null;
    for (const p of patrols) {
      const currentTotal = patrolTotals[p.patrol_id];
      const prevTotal = previousTotals[p.patrol_id];
      if (currentTotal != null && prevTotal != null) {
        const delta = currentTotal - prevTotal;
        if (best === null || delta > best.delta) {
          best = { id: p.patrol_id, delta };
        }
      }
    }
    return best?.id ?? null;
  }, [patrols, patrolTotals, previousTotals, previousTotalsLoaded, hasPreviousSession]);

  // Get the effective award value (saved selection, or auto-calculated suggestion)
  const getAwardValue = useCallback(
    (awardType: string): string => {
      if (awards[awardType]) return awards[awardType];
      if (awardType === 'best_patrol') return suggestedBestPatrol ?? '';
      if (awardType === 'most_improved') return suggestedMostImproved ?? '';
      return '';
    },
    [awards, suggestedBestPatrol, suggestedMostImproved],
  );

  // Save award selection (incremental)
  const handleAwardChange = useCallback(
    async (awardType: string, patrolId: string) => {
      if (!sessionId) return;
      setAwards((prev) => ({ ...prev, [awardType]: patrolId }));
      try {
        await api.saveAward(sessionId, awardType, patrolId);
      } catch (err) {
        console.error('[award] Save failed:', err);
        // Don't show error for background save — it'll retry on finalise
      }
    },
    [sessionId],
  );

  // Auto-save awards when suggestions become available and no explicit choice exists
  useEffect(() => {
    if (!sessionId || !hasAwards) return;

    if (session?.award_best_patrol && suggestedBestPatrol && !awards.best_patrol) {
      api.saveAward(sessionId, 'best_patrol', suggestedBestPatrol).catch(() => {});
    }
    if (session?.award_most_improved && suggestedMostImproved && !awards.most_improved) {
      api.saveAward(sessionId, 'most_improved', suggestedMostImproved).catch(() => {});
    }
  }, [sessionId, hasAwards, session, suggestedBestPatrol, suggestedMostImproved, awards]);

  // Progress tracking
  const submittedPatrolIds = useMemo(
    () => new Set(submissions.map((s) => s.patrol_id)),
    [submissions],
  );

  const isCurrentSubmitted = currentPatrol
    ? submittedPatrolIds.has(currentPatrol.patrol_id)
    : false;

  const allSubmitted = patrols.length > 0 &&
    every(patrols, (p) => submittedPatrolIds.has(p.patrol_id));

  // Incomplete-scores confirmation
  const [showConfirmFinalise, setShowConfirmFinalise] = useState(false);

  const incompletePatrols = useMemo(() => {
    if (!patrols.length || !criteria.length) return [];
    return patrols.filter((p) => {
      if (submittedPatrolIds.has(p.patrol_id)) return false;
      return !isPatrolComplete(p.patrol_id);
    });
  }, [patrols, criteria, submittedPatrolIds, isPatrolComplete]);

  // Active editors across all patrols (other users currently editing, not just viewing)
  const activeEditors = useMemo(() => {
    const editors: { user_name: string; patrol_name: string }[] = [];
    for (const patrol of patrols) {
      const patrolPresence = presenceMap[patrol.patrol_id] ?? {};
      for (const [uid, entry] of Object.entries(patrolPresence)) {
        if (uid === user?.id) continue; // exclude self
        if ((Date.now() - entry.at) > 30000) continue; // stale
        if (entry.mode === 'editing') {
          editors.push({ user_name: entry.user_name, patrol_name: patrol.name });
        }
      }
    }
    return editors;
  }, [patrols, presenceMap, user?.id]);

  const requestFinalise = useCallback(() => {
    // Always show confirmation dialog — it will show relevant warnings
    setShowConfirmFinalise(true);
  }, []);

  const handleAdminUnlockFromLockScreen = useCallback(async () => {
    if (!sessionId || !user?.is_admin) return;
    setUnlockingSession(true);
    setError('');
    try {
      await api.unlockSession(sessionId);
      const refreshed = await api.getSession(sessionId);
      setSession(refreshed.session);
      setPatrols(refreshed.patrols);
      setSubmissions(refreshed.submissions);
      setLockState(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not unlock session');
    } finally {
      setUnlockingSession(false);
    }
  }, [sessionId, user?.is_admin]);

  const isLastPatrol = currentPatrolIndex === patrols.length - 1;

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  if (!session) {
    return (
      <Box p={4} textAlign="center">
        <Flash variant="danger">Session not found</Flash>
      </Box>
    );
  }

  // ─── Lock screen: another user has finalised the session ───
  if (lockState) {
    const actionDate = new Date(lockState.actionAt);
    const formattedActionAt = actionDate.toLocaleString(undefined, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
    const deadlineDate = new Date(lockState.endsAt);
    const formattedDeadline = deadlineDate.toLocaleString(undefined, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });

    return (
      <Box
        display="flex"
        flexDirection="column"
        alignItems="center"
        justifyContent="center"
        minHeight="100vh"
        bg="canvas.default"
        p={4}
      >
        <Box
          bg="canvas.subtle"
          borderRadius={2}
          borderWidth={1}
          borderStyle="solid"
          borderColor="border.default"
          p={5}
          maxWidth="480px"
          sx={{ width: '100%', textAlign: 'center' }}
        >
          {error && (
            <Flash variant="danger" sx={{ mb: 3 }}>
              {error}
            </Flash>
          )}

          <Text sx={{ fontSize: 4, display: 'block', mb: 3 }}>🔒</Text>
          <Heading sx={{ fontSize: 3, mb: 2 }}>
            {lockState.kind === 'session' ? 'Session Locked' : 'Subcamp Finalised'}
          </Heading>
          <Text as="p" sx={{ fontSize: 1, color: 'fg.muted', mb: 3 }}>
            <Text sx={{ fontWeight: 'bold' }}>{lockState.displayName}</Text>
            {lockState.kind === 'session'
              ? ' has locked this session. No further scoring edits can be made by any user.'
              : ' has submitted the final scores for your subcamp. No further edits can be made.'}
          </Text>

          <Box
            borderRadius={2}
            p={3}
            mb={3}
            sx={{ bg: 'neutral.subtle', borderWidth: 1, borderStyle: 'solid', borderColor: 'border.muted' }}
          >
            <Text as="p" sx={{ fontSize: 0, color: 'fg.muted', mb: 1, fontWeight: 'bold' }}>
              {lockState.kind === 'session' ? 'Locked at' : 'Finalised at'}
            </Text>
            <Text sx={{ fontSize: 1, mb: 2 }}>{formattedActionAt}</Text>
            <Text as="p" sx={{ fontSize: 0, color: 'fg.muted', mb: 1, fontWeight: 'bold' }}>
              Scores deadline
            </Text>
            <Text sx={{ fontSize: 1 }}>{formattedDeadline}</Text>
          </Box>

          <Text as="p" sx={{ fontSize: 0, color: 'fg.muted', mb: 4 }}>
            If you believe this was done in error, contact{' '}
            <Text sx={{ fontWeight: 'bold' }}>{lockState.displayName}</Text> or a session
            administrator. An admin can reopen the session to allow further changes.
          </Text>

          <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
            {user?.is_admin && lockState.kind === 'session' && (
              <Button
                onClick={handleAdminUnlockFromLockScreen}
                disabled={unlockingSession}
                variant="default"
                size="large"
                sx={{ width: '100%' }}
              >
                {unlockingSession ? 'Unlocking…' : 'Unlock Session'}
              </Button>
            )}
            <Button onClick={() => navigate('/')} size="large" sx={{ width: '100%' }}>
              Back to Dashboard
            </Button>
          </Box>
        </Box>
      </Box>
    );
  }

  const inlineDeadline = new Date(session.ends_at).toLocaleString(undefined, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });

  return (
    <Box display="flex" flexDirection="column" minHeight="100vh" bg="canvas.default">
      {/* Top bar */}
      <Box
        p={3}
        borderBottomWidth={1}
        borderBottomStyle="solid"
        borderBottomColor="border.default"
        bg="canvas.subtle"
      >
        <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
          <Button
            variant="invisible"
            onClick={() => {
              if (view === 'summary') {
                navigate('/');
                return;
              }
              goToSummary();
            }}
            size="small"
          >
            {view === 'summary' ? '← Back' : '← Back to Summary'}
          </Button>
          <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
            {session.event_name}
          </Text>
        </Box>
        <Heading sx={{ fontSize: 2, mb: 1 }}>
          {session.name}
          <Text as="span" sx={{ fontSize: 0, color: 'fg.subtle', fontWeight: 'normal', ml: 1 }}>
            Deadline {inlineDeadline}
          </Text>
        </Heading>

        {/* Progress bar — tracks patrols that are scored or submitted */}
        {(() => {
          const readyCount = patrols.filter(
            (p) => submittedPatrolIds.has(p.patrol_id) || isPatrolComplete(p.patrol_id),
          ).length;
          return (
            <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
              <ProgressBar
                progress={(readyCount / patrols.length) * 100}
                sx={{ flex: 1 }}
              />
              <CounterLabel>
                {readyCount}/{patrols.length}
              </CounterLabel>
            </Box>
          );
        })()}
      </Box>

      {error && (
        <Flash variant="danger" sx={{ m: 3 }}>
          {error}
        </Flash>
      )}

      {/* ─── Summary view ─── */}
      {view === 'summary' && (
        <>
          {/* Scrollable patrol list */}
          <Box flex={1} p={3} overflow="auto">
            <Heading sx={{ fontSize: 3, mb: 3 }}>Review Scores</Heading>

            <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
              {patrols.map((patrol, index) => {
                const isSubmitted = submittedPatrolIds.has(patrol.patrol_id);
                const isComplete = isPatrolComplete(patrol.patrol_id);
                const commentCount = (perUserComments[patrol.patrol_id] ?? []).filter(c => c.comment.length > 0).length;
                const patrolPresence = presenceMap[patrol.patrol_id] ?? {};
                const activeUsers = Object.values(patrolPresence).filter(p => (Date.now() - p.at) < 30000);

                return (
                  <Box
                    key={patrol.patrol_id}
                    as="button"
                    onClick={
                      isSubmitted
                        ? () => viewPatrolScores(index)
                        : () => jumpToPatrolFromSummary(index)
                    }
                    sx={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      p: 3,
                      borderWidth: 1,
                      borderStyle: 'solid',
                      borderColor: 'border.default',
                      borderRadius: 2,
                      bg: 'canvas.default',
                      cursor: 'pointer',
                      textAlign: 'left',
                      width: '100%',
                      ':hover': { bg: 'canvas.subtle' },
                    }}
                  >
                    <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                      <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>
                        {patrolLabel(patrol)}
                      </Text>
                      {commentCount > 0 && (
                        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>💬 {commentCount}</Text>
                      )}
                      {activeUsers.length > 0 && (
                        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                          👥 {activeUsers.map(u => u.user_name).join(', ')} {activeUsers.some(u => u.mode === 'editing') ? 'editing' : 'viewing'}
                        </Text>
                      )}
                      {isSubmitted && (
                        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>Tap to view</Text>
                      )}
                    </Box>
                    <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                      {patrolTotals[patrol.patrol_id] != null && (
                        <Text sx={{ fontSize: 1, color: 'fg.muted', whiteSpace: 'nowrap' }}>
                          Total: {patrolTotals[patrol.patrol_id]}/{criteria.reduce((sum, c) => sum + c.max_value, 0)}
                        </Text>
                      )}
                      {isSubmitted ? (
                        <Label variant="success">Submitted ✓</Label>
                      ) : isComplete ? (
                        <Label variant="accent">Scores set ✓</Label>
                      ) : (
                        <Label variant="attention">Incomplete</Label>
                      )}
                    </Box>
                  </Box>
                );
              })}
            </Box>
          </Box>

          {/* Fixed bottom panel: Awards + Action buttons */}
          <Box
            borderTopWidth={1}
            borderTopStyle="solid"
            borderTopColor="border.default"
            bg="canvas.subtle"
          >
            {/* Awards panel (only if session has awards enabled) */}
            {hasAwards && (
              <Box
                p={3}
                borderBottomWidth={1}
                borderBottomStyle="solid"
                borderBottomColor="border.default"
              >
                <Heading sx={{ fontSize: 1, mb: 2, color: 'fg.muted', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                  🏆 Awards
                </Heading>
                <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
                  {/* Best Patrol award */}
                  {session.award_best_patrol && (
                    <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                      <Text sx={{ fontSize: 1, fontWeight: 'bold', minWidth: '120px', flexShrink: 0 }}>
                        🥇 Best Patrol
                      </Text>
                      <Box sx={{ flex: 1 }}>
                        <select
                          value={getAwardValue('best_patrol')}
                          onChange={(e) => handleAwardChange('best_patrol', e.target.value)}
                          disabled={allSubmitted}
                          style={{
                            width: '100%',
                            padding: '8px 12px',
                            borderRadius: '6px',
                            border: '1px solid var(--borderColor-default, #d0d7de)',
                            backgroundColor: 'var(--bgColor-default, #fff)',
                            fontSize: '14px',
                            cursor: allSubmitted ? 'not-allowed' : 'pointer',
                          }}
                        >
                          <option value="">Select patrol…</option>
                          {patrols.map((p) => (
                            <option key={p.patrol_id} value={p.patrol_id}>
                              {session.round_type === 'round2' && p.subcamp ? `${p.subcamp} - ${p.name}` : p.name}
                              {patrolTotals[p.patrol_id] != null ? ` (${patrolTotals[p.patrol_id]} pts)` : ''}
                            </option>
                          ))}
                        </select>
                      </Box>
                    </Box>
                  )}

                  {/* Most Improved award */}
                  {session.award_most_improved && (
                    <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
                      <Text sx={{ fontSize: 1, fontWeight: 'bold', minWidth: '120px', flexShrink: 0 }}>
                        📈 Most Improved
                      </Text>
                      <Box sx={{ flex: 1 }}>
                        {hasPreviousSession ? (
                          <select
                            value={getAwardValue('most_improved')}
                            onChange={(e) => handleAwardChange('most_improved', e.target.value)}
                            disabled={allSubmitted}
                            style={{
                              width: '100%',
                              padding: '8px 12px',
                              borderRadius: '6px',
                              border: '1px solid var(--borderColor-default, #d0d7de)',
                              backgroundColor: 'var(--bgColor-default, #fff)',
                              fontSize: '14px',
                              cursor: allSubmitted ? 'not-allowed' : 'pointer',
                            }}
                          >
                            <option value="">Select patrol…</option>
                            {patrols.map((p) => {
                              const curr = patrolTotals[p.patrol_id];
                              const prev = previousTotals[p.patrol_id];
                              const delta = curr != null && prev != null ? curr - prev : null;
                              return (
                                <option key={p.patrol_id} value={p.patrol_id}>
                                  {p.name}
                                  {delta != null ? ` (${delta >= 0 ? '+' : ''}${delta})` : ''}
                                </option>
                              );
                            })}
                          </select>
                        ) : (
                          <Text sx={{ fontSize: 1, color: 'fg.muted', fontStyle: 'italic', py: 1 }}>
                            No previous session — not available
                          </Text>
                        )}
                      </Box>
                    </Box>
                  )}
                </Box>
              </Box>
            )}

            {/* Action buttons */}
            <Box p={3} display="flex" sx={{ gap: 2 }}>
              {allSubmitted ? (
                <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
                  <Box textAlign="center" p={2}>
                    <Text sx={{ color: 'success.fg', fontWeight: 'bold', fontSize: 2 }}>
                      🎉 All patrols scored! Great work.
                    </Text>
                  </Box>
                  {session.status === 'ACTIVE' && (
                    <Button
                      onClick={handleRevise}
                      disabled={revising}
                      sx={{ width: '100%' }}
                      size="large"
                    >
                      {revising ? 'Reopening…' : '✏️ Revise Scores'}
                    </Button>
                  )}
                </Box>
              ) : session.status === 'ACTIVE' ? (
                <Box display="flex" flexDirection="column" sx={{ flex: 1, gap: 2 }}>
                  <Box display="flex" sx={{ gap: 2 }}>
                    <Button
                      onClick={() => {
                        setView('scoring');
                        setCurrentPatrolIndex(patrols.length - 1);
                      }}
                      sx={{ flex: 1 }}
                      size="large"
                    >
                      ← Prev
                    </Button>
                    <Button
                      variant="primary"
                      onClick={requestFinalise}
                      sx={{ flex: 2 }}
                      size="large"
                      disabled={submitting}
                    >
                      {submitting ? 'Submitting…' : 'Finalise Scores'}
                    </Button>
                  </Box>
                  <Text sx={{ color: 'fg.subtle', fontSize: 0, textAlign: 'center' }}>
                    🖨️ Printable summary available when session ends
                  </Text>
                </Box>
              ) : (
                <Box textAlign="center" p={2} sx={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
                  <Text sx={{ color: 'fg.muted', fontSize: 1 }}>
                    This session is closed.
                  </Text>
                  <Button
                    as="a"
                    href={`/api/sessions/${sessionId}/report-card`}
                    target="_blank"
                    rel="noopener noreferrer"
                    size="small"
                  >
                    🖨️ View Printable Summary
                  </Button>
                </Box>
              )}
            </Box>
          </Box>

          {/* Incomplete scores confirmation overlay */}
          {showConfirmFinalise && (
            <Box
              position="fixed"
              top={0}
              left={0}
              right={0}
              bottom={0}
              bg="neutral.muted"
              display="flex"
              alignItems="center"
              justifyContent="center"
              sx={{ zIndex: 100 }}
              onClick={() => setShowConfirmFinalise(false)}
            >
              <Box
                bg="canvas.default"
                borderRadius={2}
                borderWidth={1}
                borderStyle="solid"
                borderColor={activeEditors.length > 0 || incompletePatrols.length > 0 ? 'attention.emphasis' : 'border.default'}
                p={4}
                mx={3}
                maxWidth="400px"
                sx={{ width: '100%' }}
                onClick={(e: React.MouseEvent) => e.stopPropagation()}
              >
                <Heading sx={{ fontSize: 2, mb: 2 }}>
                  {incompletePatrols.length > 0 || activeEditors.length > 0
                    ? '⚠️ Confirm Submission'
                    : 'Confirm Submission'}
                </Heading>

                {activeEditors.length > 0 && (
                  <Box
                    borderRadius={2}
                    p={3}
                    mb={3}
                    sx={{ bg: 'attention.subtle', borderWidth: 1, borderStyle: 'solid', borderColor: 'attention.muted' }}
                  >
                    <Text as="p" sx={{ fontSize: 1, fontWeight: 'bold', mb: 1 }}>
                      Other users are still editing:
                    </Text>
                    <Box as="ul" sx={{ pl: 3, mb: 0 }}>
                      {activeEditors.map((e, i) => (
                        <Box as="li" key={i} sx={{ fontSize: 1 }}>
                          <Text sx={{ fontWeight: 'bold' }}>{e.user_name}</Text>
                          <Text sx={{ color: 'fg.muted' }}> — {e.patrol_name}</Text>
                        </Box>
                      ))}
                    </Box>
                    <Text as="p" sx={{ fontSize: 0, color: 'fg.muted', mt: 2, mb: 0 }}>
                      Submitting now will lock all editors out. Their unsaved changes may be lost.
                    </Text>
                  </Box>
                )}

                {incompletePatrols.length > 0 && (
                  <>
                    <Text as="p" sx={{ fontSize: 1, mb: 2, color: 'fg.muted' }}>
                      The following patrols have unset criteria that will be submitted as <strong>zero</strong>:
                    </Text>
                    <Box as="ul" sx={{ pl: 3, mb: 3 }}>
                      {incompletePatrols.map((p) => (
                        <Box as="li" key={p.patrol_id} sx={{ fontSize: 1, mb: 1 }}>
                          <Text sx={{ fontWeight: 'bold' }}>{p.name}</Text>
                          <Text sx={{ color: 'fg.muted' }}>
                            {' '}— {(touchedMap[p.patrol_id]?.size ?? 0)}/{criteria.length} criteria set
                          </Text>
                        </Box>
                      ))}
                    </Box>
                  </>
                )}

                {incompletePatrols.length === 0 && activeEditors.length === 0 && (
                  <Text as="p" sx={{ fontSize: 1, mb: 3, color: 'fg.muted' }}>
                    All patrols have been scored. Ready to submit final scores.
                  </Text>
                )}

                <Box display="flex" sx={{ gap: 2 }}>
                  <Button
                    onClick={() => setShowConfirmFinalise(false)}
                    sx={{ flex: 1 }}
                    size="large"
                  >
                    Go Back
                  </Button>
                  <Button
                    variant="danger"
                    onClick={handleFinalise}
                    sx={{ flex: 1 }}
                    size="large"
                    disabled={submitting}
                  >
                    {submitting ? 'Submitting…' : incompletePatrols.length > 0 || activeEditors.length > 0 ? 'Submit Anyway' : 'Submit'}
                  </Button>
                </Box>
              </Box>
            </Box>
          )}
        </>
      )}

      {/* ─── Scoring view ─── */}
      {view === 'scoring' && (
        <>
          {/* Patrol selector strip */}
          <Box
            display="flex"
            alignItems="center"
            borderBottomWidth={1}
            borderBottomStyle="solid"
            borderBottomColor="border.default"
          >
            {/* Scrollable patrol list */}
            <Box
              display="flex"
              overflowX="auto"
              p={2}
              sx={{
                gap: 1,
                flex: 1,
                minWidth: 0,
                // Hide scrollbar but keep scrollable
                '&::-webkit-scrollbar': { display: 'none' },
                scrollbarWidth: 'none',
              }}
            >
              {patrols.map((patrol, index) => {
                const isSubmitted = submittedPatrolIds.has(patrol.patrol_id);
                const isComplete = isPatrolComplete(patrol.patrol_id);
                const isCurrent = index === currentPatrolIndex;
                const patrolCommentCount = (perUserComments[patrol.patrol_id] ?? []).filter(c => c.comment.length > 0).length;
                const patrolPresence = presenceMap[patrol.patrol_id] ?? {};
                const activeUsers = Object.values(patrolPresence).filter(p => (Date.now() - p.at) < 30000);
                const hasPresence = activeUsers.length > 0;
                const anyEditing = activeUsers.some(p => p.mode === 'editing');

                return (
                  <Button
                    key={patrol.patrol_id}
                    variant={isCurrent ? 'primary' : 'invisible'}
                    size="small"
                    onClick={() => goToPatrol(index)}
                    sx={{
                      flexShrink: 0,
                      position: 'relative',
                    }}
                    title={hasPresence ? activeUsers.map(u => `${u.user_name} ${u.mode}`).join(', ') : undefined}
                  >
                    {patrolLabel(patrol)}
                    {patrolCommentCount > 0 && (
                      <Text sx={{ ml: 1, fontSize: 0, opacity: 0.8 }}>💬{patrolCommentCount}</Text>
                    )}
                    {hasPresence && !isCurrent && (
                      <Box
                        sx={{
                          position: 'absolute',
                          top: '-2px',
                          right: '-2px',
                          width: '8px',
                          height: '8px',
                          borderRadius: '50%',
                          bg: anyEditing ? 'success.emphasis' : 'attention.emphasis',
                          border: '2px solid',
                          borderColor: 'canvas.default',
                        }}
                      />
                    )}
                    {(isSubmitted || isComplete) && (
                      <Label
                        variant={isSubmitted ? 'success' : 'accent'}
                        sx={{ ml: 1 }}
                      >
                        ✓
                      </Label>
                    )}
                  </Button>
                );
              })}
            </Box>

            {/* Pinned Summary button */}
            <Box
              p={2}
              pl={1}
              sx={{
                flexShrink: 0,
                borderLeftWidth: 1,
                borderLeftStyle: 'solid',
                borderLeftColor: 'border.default',
              }}
            >
              <Button size="small" onClick={goToSummary}>
                Summary →
              </Button>
            </Box>
          </Box>

          {/* Scoring area */}
          {currentPatrol && (
            <Box flex={1} p={3} overflow="auto">
              <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
                <Heading sx={{ fontSize: 3 }}>{patrolTitle(currentPatrol)}</Heading>
                {isCurrentSubmitted && (
                  <Label variant="success" size="large">Submitted ✓</Label>
                )}
              </Box>

              {/* Live multiplayer: show who else is present on this patrol */}
              {(() => {
                const patrolPresence = presenceMap[currentPatrol.patrol_id] ?? {};
                const activeUsers = Object.values(patrolPresence).filter(p => (Date.now() - p.at) < 30000);
                if (activeUsers.length === 0) return null;
                const anyEditing = activeUsers.some(p => p.mode === 'editing');
                const names = activeUsers.map(u => u.user_name);
                const nameStr = names.length === 1
                  ? names[0]
                  : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
                const verb = anyEditing ? 'editing' : 'viewing';
                return (
                  <Flash variant={anyEditing ? 'warning' : 'default'} sx={{ mb: 3, py: 2, px: 3 }}>
                    <Text sx={{ fontSize: 1 }}>
                      👥 <strong>{nameStr}</strong> {activeUsers.length === 1 ? 'is' : 'are'} also {verb} this patrol
                    </Text>
                  </Flash>
                );
              })()}

              {/* Criteria sliders */}
              <Box display="flex" flexDirection="column" sx={{ gap: 3 }}>
                {criteria.map((criterion, index) => {
                  // Other users' comments for this criterion
                  const otherComments = (perUserComments[currentPatrol.patrol_id] ?? [])
                    .filter((c) => c.criterion_id === criterion.id && c.user_id !== user?.id);

                  // Commenting indicator: who else is typing on this criterion
                  const patrolPresence = presenceMap[currentPatrol.patrol_id] ?? {};
                  const commenters = Object.values(patrolPresence)
                    .filter((p) => p.commenting_on === criterion.id && (Date.now() - p.at) < 30000);
                  const commentingIndicator = commenters.length > 0
                    ? `✏️ ${commenters.map((c) => c.user_name).join(', ')} ${commenters.length === 1 ? 'is' : 'are'} commenting`
                    : undefined;

                  return (
                    <ScoreSlider
                      key={criterion.id}
                      criterion={criterion}
                      value={scores[criterion.id] ?? null}
                      comment={comments[criterion.id] ?? ''}
                      isFirst={index === 0}
                      otherComments={otherComments}
                      commentingIndicator={commentingIndicator}
                      onChange={(value) => handleScoreChange(criterion.id, value)}
                      onCommentChange={(comment) => handleCommentChange(criterion.id, comment)}
                      onCommentDelete={() => handleCommentDelete(criterion.id)}
                      onCommentFocus={() => handleCommentFocus(criterion.id)}
                      onCommentBlur={handleCommentBlur}
                      disabled={isCurrentSubmitted || session.status !== 'ACTIVE'}
                    />
                  );
                })}
              </Box>
            </Box>
          )}

          {/* Bottom navigation bar */}
          <Box
            p={3}
            borderTopWidth={1}
            borderTopStyle="solid"
            borderTopColor="border.default"
            bg="canvas.subtle"
            display="flex"
            sx={{ gap: 2 }}
          >
            {jumpedFromSummary ? (
              /* Jumped from summary — single "Back to Summary" button */
              <Button
                variant="primary"
                onClick={goToSummary}
                sx={{ flex: 1 }}
                size="large"
              >
                ← Back to Summary
              </Button>
            ) : (
              /* Normal Prev / Next flow */
              <>
                <Button
                  onClick={goPrev}
                  disabled={currentPatrolIndex === 0}
                  sx={{ flex: 1 }}
                  size="large"
                >
                  ← Prev
                </Button>

                <Button
                  onClick={goNext}
                  sx={{ flex: 1 }}
                  size="large"
                >
                  {isLastPatrol ? 'Review →' : 'Next →'}
                </Button>
              </>
            )}
          </Box>
        </>
      )}

      {/* ─── Viewing submitted scores (read-only) ─── */}
      {view === 'viewing' && currentPatrol && (
        <>
          <Box flex={1} p={3} overflow="auto">
            <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
              <Heading sx={{ fontSize: 3 }}>{patrolTitle(currentPatrol)}</Heading>
              <Label variant="success" size="large">Submitted ✓</Label>
            </Box>

            <Box display="flex" flexDirection="column" sx={{ gap: 3 }}>
              {criteria.map((criterion, index) => {
                const submittedComments = viewingPerUserComments
                  .filter((c) => c.criterion_id === criterion.id);
                // Use the first comment as the "main" comment display, all go into otherComments
                return (
                  <ScoreSlider
                    key={criterion.id}
                    criterion={criterion}
                    value={viewingScores[criterion.id] ?? null}
                    comment=""
                    isFirst={index === 0}
                    otherComments={submittedComments}
                    onChange={() => {}}
                    onCommentChange={() => {}}
                    disabled
                  />
                );
              })}
            </Box>
          </Box>

          <Box
            p={3}
            borderTopWidth={1}
            borderTopStyle="solid"
            borderTopColor="border.default"
            bg="canvas.subtle"
          >
            <Button
              variant="primary"
              onClick={() => setView('summary')}
              sx={{ width: '100%' }}
              size="large"
            >
              ← Back to Summary
            </Button>
          </Box>
        </>
      )}

    </Box>
  );
};
