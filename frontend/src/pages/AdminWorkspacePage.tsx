import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Box, Heading, Text, Spinner, Flash, Button, Label, TextInput,
} from '@primer/react';
import { useAuth } from '../hooks/useAuth';
import * as api from '../lib/api';

const sectionStyle = {
  borderWidth: 1,
  borderStyle: 'solid',
  borderColor: 'border.default',
  borderRadius: 2,
  bg: 'canvas.default',
} as const;

const formatLocalInput = (date = new Date()) => {
  const offset = date.getTimezoneOffset();
  const local = new Date(date.getTime() - offset * 60_000);
  return local.toISOString().slice(0, 16);
};

const Section = ({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) => (
  <Box p={3} {...sectionStyle}>
    <Box mb={3}>
      <Heading sx={{ fontSize: 2, mb: 1 }}>{title}</Heading>
      <Text sx={{ color: 'fg.muted', fontSize: 1 }}>{description}</Text>
    </Box>
    {children}
  </Box>
);

const fieldWrapStyle = { display: 'grid', gap: 6 } as const;
const controlStyle = { width: '100%' } as const;
const textareaStyle = {
  width: '100%',
  minHeight: '80px',
  borderRadius: '6px',
  border: '1px solid var(--borderColor-default, #d0d7de)',
  background: 'var(--bgColor-default, #fff)',
  color: 'var(--fgColor-default, #24292f)',
  padding: '8px 12px',
  font: 'inherit',
} as const;

export const AdminWorkspacePage = () => {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const [bootstrap, setBootstrap] = useState<api.AdminBootstrap | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const [userForm, setUserForm] = useState({
    username: '',
    displayName: '',
    password: '',
    isAdmin: false,
  });
  const [passwordForm, setPasswordForm] = useState({
    userId: '',
    password: '',
  });
  const [assignForm, setAssignForm] = useState({
    userId: '',
    patrolId: '',
    sortOrder: '',
  });
  const [eventForm, setEventForm] = useState({ name: '', description: '' });
  const [templateForm, setTemplateForm] = useState({ name: '', description: '' });
  const [criterionForm, setCriterionForm] = useState({
    templateId: '',
    title: '',
    description: '',
    minValue: '0',
    maxValue: '10',
    sortOrder: '',
  });
  const [patrolForm, setPatrolForm] = useState({ name: '' });
  const [sessionCreateForm, setSessionCreateForm] = useState({
    eventId: '',
    templateId: '',
    name: '',
    start: formatLocalInput(),
    duration: '3h',
    previousSessionId: '',
    awardBestPatrol: false,
    awardMostImproved: false,
  });
  const [sessionUpdateForm, setSessionUpdateForm] = useState({
    sessionId: '',
    previousSessionId: '',
    awardBestPatrol: false,
    awardMostImproved: false,
  });
  const [seedForm, setSeedForm] = useState({
    sessionId: '',
    userId: '',
    minScore: '3',
    maxScore: '10',
  });

  const loadBootstrap = useCallback(async () => {
    const data = await api.getAdminBootstrap();
    setBootstrap(data);
    return data;
  }, []);

  useEffect(() => {
    loadBootstrap()
      .catch((err) => setError(err instanceof Error ? err.message : 'Could not load admin data'))
      .finally(() => setLoading(false));
  }, [loadBootstrap]);

  useEffect(() => {
    if (!bootstrap) return;

    setPasswordForm((prev) => ({
      userId: prev.userId || bootstrap.users[0]?.id || '',
      password: prev.password,
    }));
    setAssignForm((prev) => ({
      userId: prev.userId || bootstrap.users[0]?.id || '',
      patrolId: prev.patrolId || bootstrap.patrols[0]?.id || '',
      sortOrder: prev.sortOrder,
    }));
    setCriterionForm((prev) => ({
      ...prev,
      templateId: prev.templateId || bootstrap.templates[0]?.id || '',
    }));
    setSessionCreateForm((prev) => ({
      ...prev,
      eventId: prev.eventId || bootstrap.events[0]?.id || '',
      templateId: prev.templateId || bootstrap.templates[0]?.id || '',
    }));
    setSessionUpdateForm((prev) => ({
      ...prev,
      sessionId: prev.sessionId || bootstrap.sessions[0]?.id || '',
      previousSessionId: prev.previousSessionId || '',
    }));
    setSeedForm((prev) => ({
      ...prev,
      sessionId: prev.sessionId || bootstrap.sessions[0]?.id || '',
      userId: prev.userId || bootstrap.users[0]?.id || '',
    }));
  }, [bootstrap]);

  const sortedSessions = useMemo(
    () => [...(bootstrap?.sessions ?? [])].sort((a, b) => a.name.localeCompare(b.name)),
    [bootstrap],
  );

  const activeSessions = useMemo(
    () => (bootstrap?.sessions ?? []).filter((session) => session.status === 'ACTIVE'),
    [bootstrap],
  );

  const runAction = useCallback(async (label: string, action: () => Promise<void>) => {
    setError('');
    setNotice('');
    setBusy(true);
    try {
      await action();
      await loadBootstrap();
      setNotice(label);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Request failed');
    } finally {
      setBusy(false);
    }
  }, [loadBootstrap]);

  const handleCreateUser = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('User created', async () => {
      await api.createAdminUser({
        username: userForm.username.trim(),
        display_name: userForm.displayName.trim() || userForm.username.trim(),
        password: userForm.password,
        is_admin: userForm.isAdmin,
      });
      setUserForm((prev) => ({ ...prev, username: '', displayName: '', password: '' }));
    });
  };

  const handleChangePassword = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Password updated', async () => {
      await api.changeAdminUserPassword(passwordForm.userId, passwordForm.password);
      setPasswordForm((prev) => ({ ...prev, password: '' }));
    });
  };

  const handleCreateEvent = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Event created', async () => {
      await api.createAdminEvent({
        name: eventForm.name.trim(),
        description: eventForm.description.trim() || eventForm.name.trim(),
      });
      setEventForm({ name: '', description: '' });
    });
  };

  const handleCreateTemplate = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Template created', async () => {
      await api.createAdminTemplate({
        name: templateForm.name.trim(),
        description: templateForm.description.trim() || templateForm.name.trim(),
      });
      setTemplateForm({ name: '', description: '' });
    });
  };

  const handleAddCriterion = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Criterion added', async () => {
      await api.addAdminCriterion(criterionForm.templateId, {
        title: criterionForm.title.trim(),
        description: criterionForm.description.trim() || criterionForm.title.trim(),
        min_value: Number(criterionForm.minValue),
        max_value: Number(criterionForm.maxValue),
        sort_order: criterionForm.sortOrder ? Number(criterionForm.sortOrder) : 0,
      });
      setCriterionForm((prev) => ({
        ...prev,
        title: '',
        description: '',
        minValue: '0',
        maxValue: '10',
        sortOrder: '',
      }));
    });
  };

  const handleCreatePatrol = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Patrol created', async () => {
      await api.createAdminPatrol({ name: patrolForm.name.trim() });
      setPatrolForm({ name: '' });
    });
  };

  const handleAssignPatrol = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Patrol assigned', async () => {
      await api.assignAdminPatrol(assignForm.userId, {
        patrol_id: assignForm.patrolId,
        sort_order: assignForm.sortOrder ? Number(assignForm.sortOrder) : 0,
      });
      setAssignForm((prev) => ({ ...prev, sortOrder: '' }));
    });
  };

  const handleCreateSession = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Session created', async () => {
      await api.createAdminSession({
        event_id: sessionCreateForm.eventId,
        template_id: sessionCreateForm.templateId,
        name: sessionCreateForm.name.trim(),
        start: sessionCreateForm.start ? new Date(sessionCreateForm.start).toISOString() : 'now',
        duration: sessionCreateForm.duration,
        award_best_patrol: sessionCreateForm.awardBestPatrol,
        award_most_improved: sessionCreateForm.awardMostImproved,
        previous_session_id: sessionCreateForm.previousSessionId,
      });
      setSessionCreateForm((prev) => ({
        ...prev,
        name: '',
        start: formatLocalInput(),
        duration: '3h',
      }));
    });
  };

  const handleUpdateSession = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Session updated', async () => {
      await api.updateAdminSession(sessionUpdateForm.sessionId, {
        award_best_patrol: sessionUpdateForm.awardBestPatrol,
        award_most_improved: sessionUpdateForm.awardMostImproved,
        previous_session_id: sessionUpdateForm.previousSessionId,
      });
    });
  };

  const handleSeedScores = async (e: FormEvent) => {
    e.preventDefault();
    await runAction('Scores seeded', async () => {
      await api.seedAdminSessionScores(seedForm.sessionId, {
        user_id: seedForm.userId,
        min_score: Number(seedForm.minScore),
        max_score: Number(seedForm.maxScore),
      });
    });
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  if (!bootstrap) {
    return (
      <Box p={4} textAlign="center">
        <Flash variant="danger">{error || 'Admin data not available'}</Flash>
      </Box>
    );
  }

  const sessionNames = sortedSessions.length === 0 ? [] : sortedSessions;

  return (
    <Box p={4} maxWidth="1200px" mx="auto">
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={4}>
        <Box>
          <Heading sx={{ fontSize: 4 }}>🛡️ Admin workspace</Heading>
          <Text sx={{ color: 'fg.muted', fontSize: 1 }}>
            Fix data, recover from mistakes, and keep camp moving.
          </Text>
        </Box>
        <Box display="flex" alignItems="center" sx={{ gap: 2 }}>
          {user?.is_admin && <Label variant="success">Admin</Label>}
          <Button variant="invisible" onClick={() => navigate('/')} size="small">
            Dashboard
          </Button>
          <Button variant="invisible" onClick={logout} size="small">
            Sign out
          </Button>
        </Box>
      </Box>

      {error && (
        <Flash variant="danger" sx={{ mb: 3 }}>
          {error}
        </Flash>
      )}

      {notice && (
        <Flash variant="success" sx={{ mb: 3 }}>
          {notice}
        </Flash>
      )}

      <Box display="grid" sx={{ gap: 3 }}>
        <Box display="grid" sx={{ gap: 3, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
          <Box p={3} {...sectionStyle}>
            <Text sx={{ color: 'fg.muted', fontSize: 0 }}>Users</Text>
            <Heading sx={{ fontSize: 3 }}>{bootstrap.users.length}</Heading>
          </Box>
          <Box p={3} {...sectionStyle}>
            <Text sx={{ color: 'fg.muted', fontSize: 0 }}>Sessions</Text>
            <Heading sx={{ fontSize: 3 }}>{bootstrap.sessions.length}</Heading>
          </Box>
          <Box p={3} {...sectionStyle}>
            <Text sx={{ color: 'fg.muted', fontSize: 0 }}>Active sessions</Text>
            <Heading sx={{ fontSize: 3 }}>{activeSessions.length}</Heading>
          </Box>
          <Box p={3} {...sectionStyle}>
            <Text sx={{ color: 'fg.muted', fontSize: 0 }}>Patrols</Text>
            <Heading sx={{ fontSize: 3 }}>{bootstrap.patrols.length}</Heading>
          </Box>
        </Box>

        <Section
          title="Session interventions"
          description="Create sessions, adjust award flags or previous-session links, and seed replacement scores when something needs fixing quickly."
        >
          <Box mb={3} overflow="auto">
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ textAlign: 'left', color: 'var(--fgColor-muted, #57606a)' }}>
                  <th style={{ padding: '8px 0' }}>Session</th>
                  <th>Status</th>
                  <th>Template</th>
                  <th>Flags</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {sessionNames.map((session) => (
                  <tr key={session.id} style={{ borderTop: '1px solid var(--borderColor-default, #d0d7de)' }}>
                    <td style={{ padding: '10px 0' }}>
                      <Text sx={{ fontWeight: 'bold' }}>{session.name}</Text>
                      <Text sx={{ display: 'block', color: 'fg.muted', fontSize: 0 }}>
                        {session.event_name}
                      </Text>
                    </td>
                    <td>
                      <Label variant={session.status === 'ACTIVE' ? 'success' : session.status === 'UPCOMING' ? 'accent' : 'default'}>
                        {session.status}
                      </Label>
                    </td>
                    <td>
                      <Text sx={{ fontSize: 0 }}>{session.template_name}</Text>
                    </td>
                    <td>
                      <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                        {session.award_best_patrol ? 'Best' : 'No best'} · {session.award_most_improved ? 'Improved' : 'No improved'}
                      </Text>
                    </td>
                    <td style={{ textAlign: 'right' }}>
                      <Button
                        variant="invisible"
                        size="small"
                        onClick={() => navigate(`/admin/sessions/${session.id}`)}
                      >
                        Open progress
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Box>

          <Box display="grid" sx={{ gap: 3, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }}>
            <form onSubmit={handleCreateSession}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Create session</Heading>
                <Box sx={fieldWrapStyle}>
                  <select
                    style={controlStyle}
                    value={sessionCreateForm.eventId}
                    onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, eventId: e.target.value }))}
                  >
                    {bootstrap.events.map((event) => (
                      <option key={event.id} value={event.id}>{event.name}</option>
                    ))}
                  </select>
                  <select
                    style={controlStyle}
                    value={sessionCreateForm.templateId}
                    onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, templateId: e.target.value }))}
                  >
                    {bootstrap.templates.map((template) => (
                      <option key={template.id} value={template.id}>{template.name}</option>
                    ))}
                  </select>
                  <TextInput
                    value={sessionCreateForm.name}
                    onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, name: e.target.value }))}
                    placeholder="Session name"
                    block
                  />
                  <TextInput
                    type="datetime-local"
                    value={sessionCreateForm.start}
                    onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, start: e.target.value }))}
                    block
                  />
                  <TextInput
                    value={sessionCreateForm.duration}
                    onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, duration: e.target.value }))}
                    placeholder="Duration, e.g. 3h"
                    block
                  />
                  <select
                    style={controlStyle}
                    value={sessionCreateForm.previousSessionId}
                    onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, previousSessionId: e.target.value }))}
                  >
                    <option value="">No previous session</option>
                    {bootstrap.sessions.map((session) => (
                      <option key={session.id} value={session.id}>{session.name}</option>
                    ))}
                  </select>
                  <Box display="flex" sx={{ gap: 3 }}>
                    <label><input type="checkbox" checked={sessionCreateForm.awardBestPatrol} onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, awardBestPatrol: e.target.checked }))} /> Best patrol</label>
                    <label><input type="checkbox" checked={sessionCreateForm.awardMostImproved} onChange={(e) => setSessionCreateForm((prev) => ({ ...prev, awardMostImproved: e.target.checked }))} /> Most improved</label>
                  </Box>
                  <Button type="submit" variant="primary" disabled={busy || bootstrap.events.length === 0 || bootstrap.templates.length === 0}>
                    Create session
                  </Button>
                </Box>
              </Box>
            </form>

            <form onSubmit={handleUpdateSession}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Update session</Heading>
                <Box sx={fieldWrapStyle}>
                  <select
                    style={controlStyle}
                    value={sessionUpdateForm.sessionId}
                    onChange={(e) => setSessionUpdateForm((prev) => ({ ...prev, sessionId: e.target.value }))}
                  >
                    {bootstrap.sessions.map((session) => (
                      <option key={session.id} value={session.id}>{session.name}</option>
                    ))}
                  </select>
                  <select
                    style={controlStyle}
                    value={sessionUpdateForm.previousSessionId}
                    onChange={(e) => setSessionUpdateForm((prev) => ({ ...prev, previousSessionId: e.target.value }))}
                  >
                    <option value="">Clear previous session</option>
                    {bootstrap.sessions.map((session) => (
                      <option key={session.id} value={session.id}>{session.name}</option>
                    ))}
                  </select>
                  <Box display="flex" sx={{ gap: 3 }}>
                    <label><input type="checkbox" checked={sessionUpdateForm.awardBestPatrol} onChange={(e) => setSessionUpdateForm((prev) => ({ ...prev, awardBestPatrol: e.target.checked }))} /> Best patrol</label>
                    <label><input type="checkbox" checked={sessionUpdateForm.awardMostImproved} onChange={(e) => setSessionUpdateForm((prev) => ({ ...prev, awardMostImproved: e.target.checked }))} /> Most improved</label>
                  </Box>
                  <Button type="submit" variant="primary" disabled={busy || bootstrap.sessions.length === 0}>
                    Save session settings
                  </Button>
                </Box>
              </Box>
            </form>

            <form onSubmit={handleSeedScores}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Seed scores</Heading>
                <Box sx={fieldWrapStyle}>
                  <select
                    style={controlStyle}
                    value={seedForm.sessionId}
                    onChange={(e) => setSeedForm((prev) => ({ ...prev, sessionId: e.target.value }))}
                  >
                    {bootstrap.sessions.map((session) => (
                      <option key={session.id} value={session.id}>{session.name}</option>
                    ))}
                  </select>
                  <select
                    style={controlStyle}
                    value={seedForm.userId}
                    onChange={(e) => setSeedForm((prev) => ({ ...prev, userId: e.target.value }))}
                  >
                    {bootstrap.users.map((adminUser) => (
                      <option key={adminUser.id} value={adminUser.id}>{adminUser.display_name}</option>
                    ))}
                  </select>
                  <Box display="flex" sx={{ gap: 2 }}>
                    <TextInput
                      type="number"
                      value={seedForm.minScore}
                      onChange={(e) => setSeedForm((prev) => ({ ...prev, minScore: e.target.value }))}
                      placeholder="Min"
                      block
                    />
                    <TextInput
                      type="number"
                      value={seedForm.maxScore}
                      onChange={(e) => setSeedForm((prev) => ({ ...prev, maxScore: e.target.value }))}
                      placeholder="Max"
                      block
                    />
                  </Box>
                  <Button type="submit" variant="primary" disabled={busy || bootstrap.sessions.length === 0 || bootstrap.users.length === 0}>
                    Seed submissions
                  </Button>
                </Box>
              </Box>
            </form>
          </Box>
        </Section>

        <Section
          title="Users and patrol ownership"
          description="Create accounts, reset passwords, and repair patrol assignments without touching the database by hand."
        >
          <Box display="grid" sx={{ gap: 3, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }} mb={3}>
            <form onSubmit={handleCreateUser}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Create user</Heading>
                <Box sx={fieldWrapStyle}>
                  <TextInput
                    value={userForm.username}
                    onChange={(e) => setUserForm((prev) => ({ ...prev, username: e.target.value }))}
                    placeholder="Username"
                    block
                  />
                  <TextInput
                    value={userForm.displayName}
                    onChange={(e) => setUserForm((prev) => ({ ...prev, displayName: e.target.value }))}
                    placeholder="Display name"
                    block
                  />
                  <TextInput
                    type="password"
                    value={userForm.password}
                    onChange={(e) => setUserForm((prev) => ({ ...prev, password: e.target.value }))}
                    placeholder="Temporary password"
                    block
                  />
                  <label><input type="checkbox" checked={userForm.isAdmin} onChange={(e) => setUserForm((prev) => ({ ...prev, isAdmin: e.target.checked }))} /> Admin access</label>
                  <Button type="submit" variant="primary" disabled={busy || !userForm.username || !userForm.password}>
                    Create user
                  </Button>
                </Box>
              </Box>
            </form>

            <form onSubmit={handleChangePassword}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Reset password</Heading>
                <Box sx={fieldWrapStyle}>
                  <select
                    style={controlStyle}
                    value={passwordForm.userId}
                    onChange={(e) => setPasswordForm((prev) => ({ ...prev, userId: e.target.value }))}
                  >
                    {bootstrap.users.map((adminUser) => (
                      <option key={adminUser.id} value={adminUser.id}>{adminUser.display_name} ({adminUser.username})</option>
                    ))}
                  </select>
                  <TextInput
                    type="password"
                    value={passwordForm.password}
                    onChange={(e) => setPasswordForm((prev) => ({ ...prev, password: e.target.value }))}
                    placeholder="New password"
                    block
                  />
                  <Button type="submit" variant="primary" disabled={busy || !passwordForm.userId || !passwordForm.password}>
                    Update password
                  </Button>
                </Box>
              </Box>
            </form>

            <form onSubmit={handleAssignPatrol}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Repair patrol assignment</Heading>
                <Box sx={fieldWrapStyle}>
                  <select
                    style={controlStyle}
                    value={assignForm.userId}
                    onChange={(e) => setAssignForm((prev) => ({ ...prev, userId: e.target.value }))}
                  >
                    {bootstrap.users.map((adminUser) => (
                      <option key={adminUser.id} value={adminUser.id}>{adminUser.display_name}</option>
                    ))}
                  </select>
                  <select
                    style={controlStyle}
                    value={assignForm.patrolId}
                    onChange={(e) => setAssignForm((prev) => ({ ...prev, patrolId: e.target.value }))}
                  >
                    {bootstrap.patrols.map((patrol) => (
                      <option key={patrol.id} value={patrol.id}>{patrol.name}</option>
                    ))}
                  </select>
                  <TextInput
                    type="number"
                    value={assignForm.sortOrder}
                    onChange={(e) => setAssignForm((prev) => ({ ...prev, sortOrder: e.target.value }))}
                    placeholder="Order (blank = append)"
                    block
                  />
                  <Button type="submit" variant="primary" disabled={busy || !assignForm.userId || !assignForm.patrolId}>
                    Save assignment
                  </Button>
                </Box>
              </Box>
            </form>
          </Box>

          <Box overflow="auto">
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ textAlign: 'left', color: 'var(--fgColor-muted, #57606a)' }}>
                  <th style={{ padding: '8px 0' }}>User</th>
                  <th>Admin</th>
                  <th>Assigned patrols</th>
                </tr>
              </thead>
              <tbody>
                {bootstrap.users.map((adminUser) => (
                  <tr key={adminUser.id} style={{ borderTop: '1px solid var(--borderColor-default, #d0d7de)' }}>
                    <td style={{ padding: '10px 0' }}>
                      <Text sx={{ fontWeight: 'bold' }}>{adminUser.display_name}</Text>
                      <Text sx={{ display: 'block', color: 'fg.muted', fontSize: 0 }}>
                        {adminUser.username}
                      </Text>
                    </td>
                    <td>
                      <Label variant={adminUser.is_admin ? 'success' : 'default'}>
                        {adminUser.is_admin ? 'Yes' : 'No'}
                      </Label>
                    </td>
                    <td>
                      <Box display="flex" flexWrap="wrap" sx={{ gap: 1 }}>
                        {adminUser.patrols.length > 0 ? adminUser.patrols.map((patrol) => (
                          <Label key={patrol.patrol_id} variant="accent">
                            {patrol.patrol_name}
                          </Label>
                        )) : <Text sx={{ color: 'fg.muted', fontSize: 0 }}>None</Text>}
                      </Box>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Box>
        </Section>

        <Section
          title="Events, templates, and patrol setup"
          description="Build the data structures behind a camp before the session starts."
        >
          <Box display="grid" sx={{ gap: 3, gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }} mb={3}>
            <form onSubmit={handleCreateEvent}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Create event</Heading>
                <Box sx={fieldWrapStyle}>
                  <TextInput
                    value={eventForm.name}
                    onChange={(e) => setEventForm((prev) => ({ ...prev, name: e.target.value }))}
                    placeholder="Event name"
                    block
                  />
                  <textarea
                    style={textareaStyle}
                    value={eventForm.description}
                    onChange={(e) => setEventForm((prev) => ({ ...prev, description: e.target.value }))}
                    placeholder="Description"
                  />
                  <Button type="submit" variant="primary" disabled={busy || !eventForm.name}>
                    Create event
                  </Button>
                </Box>
              </Box>
            </form>

            <form onSubmit={handleCreateTemplate}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Create template</Heading>
                <Box sx={fieldWrapStyle}>
                  <TextInput
                    value={templateForm.name}
                    onChange={(e) => setTemplateForm((prev) => ({ ...prev, name: e.target.value }))}
                    placeholder="Template name"
                    block
                  />
                  <textarea
                    style={textareaStyle}
                    value={templateForm.description}
                    onChange={(e) => setTemplateForm((prev) => ({ ...prev, description: e.target.value }))}
                    placeholder="Description"
                  />
                  <Button type="submit" variant="primary" disabled={busy || !templateForm.name}>
                    Create template
                  </Button>
                </Box>
              </Box>
            </form>

            <form onSubmit={handleAddCriterion}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Add criterion</Heading>
                <Box sx={fieldWrapStyle}>
                  <select
                    style={controlStyle}
                    value={criterionForm.templateId}
                    onChange={(e) => setCriterionForm((prev) => ({ ...prev, templateId: e.target.value }))}
                  >
                    {bootstrap.templates.map((template) => (
                      <option key={template.id} value={template.id}>{template.name}</option>
                    ))}
                  </select>
                  <TextInput
                    value={criterionForm.title}
                    onChange={(e) => setCriterionForm((prev) => ({ ...prev, title: e.target.value }))}
                    placeholder="Criterion title"
                    block
                  />
                  <textarea
                    style={textareaStyle}
                    value={criterionForm.description}
                    onChange={(e) => setCriterionForm((prev) => ({ ...prev, description: e.target.value }))}
                    placeholder="Description"
                  />
                  <Box display="flex" sx={{ gap: 2 }}>
                    <TextInput
                      type="number"
                      value={criterionForm.minValue}
                      onChange={(e) => setCriterionForm((prev) => ({ ...prev, minValue: e.target.value }))}
                      placeholder="Min"
                      block
                    />
                    <TextInput
                      type="number"
                      value={criterionForm.maxValue}
                      onChange={(e) => setCriterionForm((prev) => ({ ...prev, maxValue: e.target.value }))}
                      placeholder="Max"
                      block
                    />
                  </Box>
                  <TextInput
                    type="number"
                    value={criterionForm.sortOrder}
                    onChange={(e) => setCriterionForm((prev) => ({ ...prev, sortOrder: e.target.value }))}
                    placeholder="Order (blank = append)"
                    block
                  />
                  <Button type="submit" variant="primary" disabled={busy || !criterionForm.templateId || !criterionForm.title}>
                    Add criterion
                  </Button>
                </Box>
              </Box>
            </form>

            <form onSubmit={handleCreatePatrol}>
              <Box p={3} {...sectionStyle}>
                <Heading sx={{ fontSize: 2, mb: 2 }}>Create patrol</Heading>
                <Box sx={fieldWrapStyle}>
                  <TextInput
                    value={patrolForm.name}
                    onChange={(e) => setPatrolForm((prev) => ({ ...prev, name: e.target.value }))}
                    placeholder="Patrol name"
                    block
                  />
                  <Button type="submit" variant="primary" disabled={busy || !patrolForm.name}>
                    Create patrol
                  </Button>
                </Box>
              </Box>
            </form>
          </Box>

          <Box overflow="auto">
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ textAlign: 'left', color: 'var(--fgColor-muted, #57606a)' }}>
                  <th style={{ padding: '8px 0' }}>Template</th>
                  <th>Criteria</th>
                </tr>
              </thead>
              <tbody>
                {bootstrap.templates.map((template) => (
                  <tr key={template.id} style={{ borderTop: '1px solid var(--borderColor-default, #d0d7de)' }}>
                    <td style={{ padding: '10px 0' }}>
                      <Text sx={{ fontWeight: 'bold' }}>{template.name}</Text>
                      <Text sx={{ display: 'block', color: 'fg.muted', fontSize: 0 }}>
                        {template.description}
                      </Text>
                    </td>
                    <td>
                      <Box display="flex" flexWrap="wrap" sx={{ gap: 1 }}>
                        {template.criteria.length > 0 ? template.criteria.map((criterion) => (
                          <Label key={criterion.id} variant="accent">
                            {criterion.title} ({criterion.min_value}-{criterion.max_value})
                          </Label>
                        )) : <Text sx={{ color: 'fg.muted', fontSize: 0 }}>No criteria yet</Text>}
                      </Box>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Box>

          <Box mt={3} overflow="auto">
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ textAlign: 'left', color: 'var(--fgColor-muted, #57606a)' }}>
                  <th style={{ padding: '8px 0' }}>Patrol</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                {bootstrap.patrols.map((patrol) => (
                  <tr key={patrol.id} style={{ borderTop: '1px solid var(--borderColor-default, #d0d7de)' }}>
                    <td style={{ padding: '10px 0' }}>
                      <Text sx={{ fontWeight: 'bold' }}>{patrol.name}</Text>
                    </td>
                    <td>
                      <Text sx={{ color: 'fg.muted', fontSize: 0 }}>{patrol.created_at}</Text>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Box>
        </Section>
      </Box>
    </Box>
  );
};
