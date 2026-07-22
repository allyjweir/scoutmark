import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Box, Button, Flash, Heading, Spinner, Text } from '@primer/react';
import type { Session } from '../lib/types';
import * as api from '../lib/api';
import { SessionCard } from '../components/SessionCard';

export const CampChiefProgressPage = () => {
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.listSessions()
      .then(({ sessions: allSessions }) => setSessions(allSessions.filter((session) => (session.round_type ?? 'regular') === 'regular')))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh"><Spinner size="large" /></Box>;
  }

  return (
    <Box p={3} maxWidth="600px" mx="auto">
      <Button variant="invisible" size="small" onClick={() => navigate('/')}>← Dashboard</Button>
      <Heading sx={{ fontSize: 3, mt: 2, mb: 1 }}>Regular scoring progress</Heading>
      <Text sx={{ color: 'fg.muted', fontSize: 1, display: 'block', mb: 4 }}>
        Read-only progress across all subcamps.
      </Text>
      {error && <Flash variant="danger" sx={{ mb: 3 }}>{error}</Flash>}
      <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
        {sessions.map((session) => (
          <SessionCard
            key={session.id}
            session={session}
            onClick={() => navigate(`/campchief/progress/${session.id}`)}
          />
        ))}
      </Box>
      {sessions.length === 0 && <Text sx={{ color: 'fg.muted' }}>No regular sessions available.</Text>}
    </Box>
  );
};
