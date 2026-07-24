import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Box, Button, Flash, FormControl, Heading, Label, Spinner, Text, TextInput } from '@primer/react';
import type { Session } from '../lib/types';
import type { AdminSubcamp, AdminUser } from '../lib/api';
import * as api from '../lib/api';
import { useAuth } from '../hooks/useAuth';

const toUTCDateTimeLocal = (value: string) => new Date(value).toISOString().slice(0, 16);
const nowUTCDateTimeLocal = () => new Date().toISOString().slice(0, 16);

const asUTCISO = (value: string) => {
  const trimmed = value.trim();
  if (trimmed.length === 16) return `${trimmed}:00Z`;
  if (trimmed.endsWith('Z')) return trimmed;
  return `${trimmed}Z`;
};

export const AdminDashboardPage = () => {
  const navigate = useNavigate();
  const { logout } = useAuth();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [subcamps, setSubcamps] = useState<AdminSubcamp[]>([]);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [sessionSubcamps, setSessionSubcamps] = useState<Record<string, AdminSubcamp[]>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [newUser, setNewUser] = useState({ username: '', display_name: '', password: '', subcamp_id: '', is_admin: false });
  const [passwordUser, setPasswordUser] = useState<AdminUser | null>(null);
  const [newPassword, setNewPassword] = useState('');
  const [round2Source, setRound2Source] = useState<Session | null>(null);
  const [round2StartsAt, setRound2StartsAt] = useState('');
  const [round2EndsAt, setRound2EndsAt] = useState('');

  const load = useCallback(async () => {
    const [sessionData, userData, subcampData] = await Promise.all([
      api.listSessions(), api.listAdminUsers(), api.listAdminSubcamps(),
    ]);
    setSessions(sessionData.sessions);
    setUsers(userData.users);
    setSubcamps(subcampData.subcamps);
  }, []);

  useEffect(() => {
    load().catch((err) => setError(err.message)).finally(() => setLoading(false));
  }, [load]);

  const toggleSession = async (sessionId: string) => {
    if (expanded === sessionId) {
      setExpanded(null);
      return;
    }
    setExpanded(sessionId);
    if (!sessionSubcamps[sessionId]) {
      try {
        const data = await api.getAdminSessionSubcamps(sessionId);
        setSessionSubcamps((previous) => ({ ...previous, [sessionId]: data.subcamps }));
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Could not load subcamps');
      }
    }
  };

  const saveTimes = async (session: Session, startsAt: string, endsAt: string) => {
    setSaving(session.id);
    setError('');
    try {
      const { session: updated } = await api.updateAdminSession(session.id, asUTCISO(startsAt), asUTCISO(endsAt));
      setSessions((items) => items.map((item) => item.id === updated.id ? updated : item));
      setNotice(`${session.name} timing updated.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not update session');
    } finally {
      setSaving(null);
    }
  };

  const changeSubcampLock = async (sessionId: string, subcamp: AdminSubcamp) => {
    setSaving(`${sessionId}:${subcamp.id}`);
    setError('');
    try {
      if (subcamp.locked_at) await api.unlockAdminSessionSubcamp(sessionId, subcamp.id);
      else await api.lockAdminSessionSubcamp(sessionId, subcamp.id);
      const data = await api.getAdminSessionSubcamps(sessionId);
      setSessionSubcamps((previous) => ({ ...previous, [sessionId]: data.subcamps }));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not update subcamp lock');
    } finally {
      setSaving(null);
    }
  };

  const changeSubcampFinalisation = async (sessionId: string, subcamp: AdminSubcamp, revise: boolean) => {
    setSaving(`${sessionId}:${subcamp.id}:${revise ? 'revise' : 'finalise'}`);
    setError('');
    try {
      if (revise) await api.reviseSession(sessionId, subcamp.id);
      else await api.finaliseSession(sessionId, subcamp.id);
      const data = await api.getAdminSessionSubcamps(sessionId);
      setSessionSubcamps((previous) => ({ ...previous, [sessionId]: data.subcamps }));
      setNotice(revise ? `${subcamp.name} reopened for revision.` : `${subcamp.name} finalised.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : `Could not ${revise ? 'revise' : 'finalise'} subcamp`);
    } finally {
      setSaving(null);
    }
  };

  const unlockSession = async (session: Session) => {
    setSaving(session.id);
    setError('');
    try {
      await api.unlockSession(session.id);
      const { sessions: updatedSessions } = await api.listSessions();
      setSessions(updatedSessions);
      setNotice(`${session.name} unlocked.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not unlock session');
    } finally {
      setSaving(null);
    }
  };

  const openRound2Creator = (session: Session) => {
    const sourceDuration = new Date(session.ends_at).getTime() - new Date(session.starts_at).getTime();
    const duration = Math.max(sourceDuration, 60 * 60 * 1000);
    const startsAt = nowUTCDateTimeLocal();
    const endsAt = new Date(Date.now() + duration).toISOString().slice(0, 16);
    setRound2Source(session);
    setRound2StartsAt(startsAt);
    setRound2EndsAt(endsAt);
    setError('');
  };

  const createRound2 = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!round2Source) return;
    setSaving('create-round2');
    setError('');
    try {
      const { session } = await api.createAdminRound2Session(
        round2Source.id,
        asUTCISO(round2StartsAt),
        asUTCISO(round2EndsAt),
      );
      setSessions((items) => [...items, session]);
      setRound2Source(null);
      setNotice(`${session.name} created. Select one finalist for each subcamp.`);
      navigate(`/admin/sessions/${session.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not create Round 2');
    } finally {
      setSaving(null);
    }
  };

  const createUser = async (event: React.FormEvent) => {
    event.preventDefault();
    setSaving('create-user');
    setError('');
    try {
      await api.createAdminUser(newUser);
      setNewUser({ username: '', display_name: '', password: '', subcamp_id: '', is_admin: false });
      const { users: updatedUsers } = await api.listAdminUsers();
      setUsers(updatedUsers);
      setNotice('Account created. The user will choose a new password when signing in.');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not create account');
    } finally {
      setSaving(null);
    }
  };

  const resetPassword = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!passwordUser) return;
    setSaving(`password:${passwordUser.id}`);
    try {
      await api.resetAdminUserPassword(passwordUser.id, newPassword);
      setNotice(`Password reset for ${passwordUser.display_name}.`);
      setPasswordUser(null);
      setNewPassword('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not reset password');
    } finally {
      setSaving(null);
    }
  };

  const newestSessions = (statuses: Session['status'][]) => sessions
    .filter((session) => statuses.includes(session.status))
    .sort((first, second) => new Date(second.created_at).getTime() - new Date(first.created_at).getTime());
  const sessionGroups = [
    { title: 'Active', sessions: newestSessions(['ACTIVE']) },
    { title: 'Closed / Locked', sessions: newestSessions(['CLOSED', 'LOCKED']) },
    { title: 'Upcoming', sessions: newestSessions(['UPCOMING']) },
  ];

  if (loading) return <Box display="flex" justifyContent="center" minHeight="100vh" alignItems="center"><Spinner size="large" /></Box>;

  return (
    <Box p={3} maxWidth="720px" mx="auto">
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={4}>
        <Box>
          <Heading sx={{ fontSize: 3 }}>Admin dashboard</Heading>
          <Text sx={{ color: 'fg.muted', fontSize: 1 }}>Sessions, scores, and accounts</Text>
        </Box>
        <Button variant="invisible" size="small" onClick={logout}>Sign out</Button>
      </Box>
      {error && <Flash variant="danger" sx={{ mb: 3 }}>{error}</Flash>}
      {notice && <Flash variant="success" sx={{ mb: 3 }}>{notice}</Flash>}

      <Heading sx={{ fontSize: 2, mb: 3 }}>Sessions</Heading>
      <Box display="flex" flexDirection="column" sx={{ gap: 4 }} mb={5}>
        {sessionGroups.map((group) => group.sessions.length > 0 && (
          <Box key={group.title}>
            <Heading sx={{ fontSize: 1, mb: 2, color: 'fg.muted' }}>{group.title}</Heading>
            <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
              {group.sessions.map((session) => (
                <SessionAdminCard
                  key={session.id}
                  session={session}
                  open={expanded === session.id}
                  subcamps={sessionSubcamps[session.id] ?? []}
                  busy={saving}
                  onToggle={() => toggleSession(session.id)}
                  onSave={saveTimes}
                  onLock={changeSubcampLock}
                  onFinalise={(sessionId, subcamp) => changeSubcampFinalisation(sessionId, subcamp, false)}
                  onRevise={(sessionId, subcamp) => changeSubcampFinalisation(sessionId, subcamp, true)}
                  onUnlockSession={unlockSession}
                  onScores={() => navigate(`/admin/sessions/${session.id}/scores`)}
                  onCreateRound2={() => openRound2Creator(session)}
                />
              ))}
            </Box>
          </Box>
        ))}
      </Box>

      {round2Source && (
        <Box as="form" onSubmit={createRound2} p={3} mb={5} borderWidth={1} borderStyle="solid" borderColor="accent.emphasis" borderRadius={2} bg="accent.subtle">
          <Heading sx={{ fontSize: 2, mb: 1 }}>Create Round 2</Heading>
          <Text sx={{ fontSize: 1, color: 'fg.muted', display: 'block', mb: 3 }}>{round2Source.name}</Text>
          <FormControl sx={{ mb: 2 }}><FormControl.Label>Opens</FormControl.Label><TextInput required type="datetime-local" value={round2StartsAt} onChange={(event) => setRound2StartsAt(event.target.value)} /></FormControl>
          <FormControl sx={{ mb: 3 }}><FormControl.Label>Closes</FormControl.Label><TextInput required type="datetime-local" value={round2EndsAt} onChange={(event) => setRound2EndsAt(event.target.value)} /></FormControl>
          <Box display="flex" sx={{ gap: 2 }}>
            <Button type="submit" disabled={saving === 'create-round2'}>{saving === 'create-round2' ? 'Creating...' : 'Create and select finalists'}</Button>
            <Button type="button" onClick={() => setRound2Source(null)} disabled={saving === 'create-round2'}>Cancel</Button>
          </Box>
        </Box>
      )}

      <Box borderTopWidth={1} borderTopStyle="solid" borderTopColor="border.default" pt={4}>
        <Heading sx={{ fontSize: 2, mb: 2 }}>Users</Heading>
        <Box display="flex" flexDirection="column" sx={{ gap: 2 }} mb={4}>
          {users.map((user) => (
            <Box key={user.id} p={2} borderWidth={1} borderStyle="solid" borderColor="border.default" borderRadius={2} display="flex" justifyContent="space-between" alignItems="center">
              <Box>
                <Text sx={{ fontWeight: 'bold', display: 'block' }}>{user.display_name}</Text>
                <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{user.username} · {user.subcamp_name ?? 'No subcamp'}{user.is_admin ? ' · Admin' : ''}</Text>
              </Box>
              <Button size="small" onClick={() => setPasswordUser(user)}>Reset password</Button>
            </Box>
          ))}
        </Box>
        <Heading sx={{ fontSize: 1, mb: 2 }}>Create account</Heading>
        <Box as="form" onSubmit={createUser} p={3} bg="canvas.subtle" borderRadius={2}>
          <FormControl sx={{ mb: 2 }}><FormControl.Label>Name</FormControl.Label><TextInput required value={newUser.display_name} onChange={(e) => setNewUser({ ...newUser, display_name: e.target.value })} /></FormControl>
          <FormControl sx={{ mb: 2 }}><FormControl.Label>Username</FormControl.Label><TextInput required value={newUser.username} onChange={(e) => setNewUser({ ...newUser, username: e.target.value })} /></FormControl>
          <FormControl sx={{ mb: 2 }}><FormControl.Label>Temporary password</FormControl.Label><TextInput required type="password" minLength={8} value={newUser.password} onChange={(e) => setNewUser({ ...newUser, password: e.target.value })} /></FormControl>
          <FormControl sx={{ mb: 2 }}><FormControl.Label>Subcamp</FormControl.Label><select required value={newUser.subcamp_id} onChange={(e) => setNewUser({ ...newUser, subcamp_id: e.target.value })}><option value="">Choose subcamp</option>{subcamps.map((subcamp) => <option key={subcamp.id} value={subcamp.id}>{subcamp.name}</option>)}</select></FormControl>
          <label><input type="checkbox" checked={newUser.is_admin} onChange={(e) => setNewUser({ ...newUser, is_admin: e.target.checked })} /> Admin account</label>
          <Box mt={3}><Button type="submit" disabled={saving === 'create-user'}>{saving === 'create-user' ? 'Creating…' : 'Create account'}</Button></Box>
        </Box>
      </Box>

      {passwordUser && <Box as="form" onSubmit={resetPassword} p={3} mt={4} borderWidth={1} borderStyle="solid" borderColor="attention.emphasis" borderRadius={2}>
        <Heading sx={{ fontSize: 1, mb: 2 }}>Reset password for {passwordUser.display_name}</Heading>
        <Text sx={{ fontSize: 0, color: 'fg.muted', display: 'block', mb: 2 }}>They will be required to change this temporary password when they sign in.</Text>
        <TextInput required type="password" minLength={8} value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
        <Box display="flex" sx={{ gap: 2 }} mt={2}><Button type="submit" disabled={saving?.startsWith('password:')}>Reset</Button><Button type="button" onClick={() => setPasswordUser(null)}>Cancel</Button></Box>
      </Box>}
    </Box>
  );
};

const SessionAdminCard = ({ session, open, subcamps, busy, onToggle, onSave, onLock, onFinalise, onRevise, onUnlockSession, onScores, onCreateRound2 }: {
  session: Session; open: boolean; subcamps: AdminSubcamp[]; busy: string | null;
  onToggle: () => void; onSave: (session: Session, starts: string, ends: string) => void;
  onLock: (sessionId: string, subcamp: AdminSubcamp) => void;
  onFinalise: (sessionId: string, subcamp: AdminSubcamp) => void;
  onUnlockSession: (session: Session) => void; onCreateRound2: () => void;
  onRevise: (sessionId: string, subcamp: AdminSubcamp) => void; onScores: () => void;
}) => {
  const sessionStartsAt = toUTCDateTimeLocal(session.starts_at);
  const sessionEndsAt = toUTCDateTimeLocal(session.ends_at);
  const [startsAt, setStartsAt] = useState(sessionStartsAt);
  const [endsAt, setEndsAt] = useState(sessionEndsAt);
  useEffect(() => { setStartsAt(sessionStartsAt); setEndsAt(sessionEndsAt); }, [sessionStartsAt, sessionEndsAt]);

  const hasScheduleChanges = startsAt !== sessionStartsAt || endsAt !== sessionEndsAt;

  const openNow = () => {
    const nextStartsAt = nowUTCDateTimeLocal();
    setStartsAt(nextStartsAt);
    onSave(session, nextStartsAt, endsAt);
  };

  const closeNow = () => {
    const nextEndsAt = nowUTCDateTimeLocal();
    setEndsAt(nextEndsAt);
    onSave(session, startsAt, nextEndsAt);
  };

  return <Box borderWidth={1} borderStyle="solid" borderColor="border.default" borderRadius={2} overflow="hidden">
    <Box p={3} display="flex" justifyContent="space-between" alignItems="center">
      <Box><Text sx={{ fontWeight: 'bold', display: 'block' }}>{session.name}</Text><Text sx={{ fontSize: 0, color: 'fg.muted' }}>{session.event_name}</Text></Box>
      <Button size="small" onClick={onToggle}>{open ? 'Hide' : 'Manage'} <Label sx={{ ml: 1 }}>{session.status}</Label></Button>
    </Box>
    {open && <Box p={3} borderTopWidth={1} borderTopStyle="solid" borderTopColor="border.default" bg="canvas.subtle">
      <FormControl sx={{ mb: 2 }}><FormControl.Label>Opens</FormControl.Label><TextInput type="datetime-local" value={startsAt} onChange={(e) => setStartsAt(e.target.value)} /></FormControl>
      <FormControl sx={{ mb: 2 }}><FormControl.Label>Closes</FormControl.Label><TextInput type="datetime-local" value={endsAt} onChange={(e) => setEndsAt(e.target.value)} /></FormControl>
      <Box mb={3} p={2} borderWidth={1} borderStyle="solid" borderColor="border.default" borderRadius={2} bg="canvas.default">
        <Text sx={{ fontSize: 0, color: 'fg.muted', display: 'block', mb: 2 }}>Quick actions</Text>
        <Box display="flex" flexWrap="wrap" sx={{ gap: 2 }}>
          <Button size="small" onClick={openNow} disabled={busy === session.id}>Open now</Button>
          <Button size="small" onClick={closeNow} disabled={busy === session.id}>Close now</Button>
          {session.status === 'LOCKED' && <Button size="small" onClick={() => onUnlockSession(session)} disabled={busy === session.id}>Unlock session</Button>}
          <Button size="small" onClick={onScores}>Edit patrol scores</Button>
          {session.round_type !== 'round2' && <Button size="small" onClick={onCreateRound2}>Create Round 2</Button>}
        </Box>
      </Box>
      {hasScheduleChanges && <Box display="flex" flexWrap="wrap" sx={{ gap: 2 }} mb={3}><Button size="small" onClick={() => onSave(session, startsAt, endsAt)} disabled={busy === session.id}>Save schedule</Button></Box>}
      <Heading sx={{ fontSize: 1, mb: 2 }}>Subcamp scoring</Heading>
      <Box display="flex" flexDirection="column" sx={{ gap: 1 }}>
        {subcamps.map((subcamp) => {
          const finalising = busy === `${session.id}:${subcamp.id}:finalise`;
          const revising = busy === `${session.id}:${subcamp.id}:revise`;
          const actionDisabled = session.status !== 'ACTIVE' || !!subcamp.locked_at || finalising || revising;
          const state = subcamp.locked_at ? 'Locked' : subcamp.finalised ? 'Finalised' : 'Open';
          return <Box key={subcamp.id} display="flex" justifyContent="space-between" alignItems="center">
            <Text sx={{ fontSize: 1 }}>{subcamp.name} ({state}){subcamp.locked_at && subcamp.locked_by ? ` by ${subcamp.locked_by}` : ''}</Text>
            <Box display="flex" sx={{ gap: 1 }}>
              <Button size="small" disabled={actionDisabled || subcamp.finalised} onClick={() => onFinalise(session.id, subcamp)}>{finalising ? 'Finalising...' : 'Finalise on behalf'}</Button>
              <Button size="small" disabled={actionDisabled || !subcamp.finalised} onClick={() => onRevise(session.id, subcamp)}>{revising ? 'Reopening...' : 'Revise'}</Button>
              <Button size="small" variant={subcamp.locked_at ? 'default' : 'danger'} disabled={busy === `${session.id}:${subcamp.id}`} onClick={() => onLock(session.id, subcamp)}>{subcamp.locked_at ? 'Unlock' : 'Lock'}</Button>
            </Box>
          </Box>;
        })}
      </Box>
    </Box>}
  </Box>;
};
