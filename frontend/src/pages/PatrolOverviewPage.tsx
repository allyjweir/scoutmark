import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Box, Button, Flash, Heading, Spinner, Text } from '@primer/react';
import * as api from '../lib/api';
import type { PatrolHistory } from '../lib/api';

export const PatrolOverviewPage = () => {
  const navigate = useNavigate();
  const [patrols, setPatrols] = useState<PatrolHistory[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    api.getPatrolHistory()
      .then(({ patrols: result }) => setPatrols(result))
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh">
        <Spinner size="large" />
      </Box>
    );
  }

  return (
    <Box p={3} maxWidth="600px" mx="auto">
      <Button variant="invisible" size="small" onClick={() => navigate('/')} sx={{ mb: 3, px: 0 }}>
        ← Back to dashboard
      </Button>
      <Heading sx={{ fontSize: 3, mb: 1 }}>Patrols</Heading>
      <Text sx={{ color: 'fg.muted', display: 'block', mb: 3 }}>
        Select a patrol to view its score history and feedback.
      </Text>

      {error && <Flash variant="danger" sx={{ mb: 3 }}>{error}</Flash>}

      {patrols.length === 0 ? (
        <Box textAlign="center" py={6}>
          <Text sx={{ color: 'fg.muted', fontSize: 2 }}>No patrols have been set up yet.</Text>
        </Box>
      ) : (
        <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
          {patrols.map((patrol) => (
            <Box
              key={patrol.patrol_id}
              as="button"
              onClick={() => navigate(`/patrols/${patrol.patrol_id}`)}
              sx={{
                width: '100%',
                p: 3,
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                textAlign: 'left',
                bg: 'canvas.default',
                borderWidth: 1,
                borderStyle: 'solid',
                borderColor: 'border.default',
                borderRadius: 2,
                cursor: 'pointer',
                ':hover': { bg: 'canvas.subtle' },
              }}
            >
              <Box>
                <Text sx={{ display: 'block', fontWeight: 'bold', fontSize: 2 }}>{patrol.name}</Text>
                <Text sx={{ color: 'fg.muted', fontSize: 0 }}>
                  {patrol.sessions.length} {patrol.sessions.length === 1 ? 'scored session' : 'scored sessions'}
                </Text>
              </Box>
              <Text sx={{ color: 'fg.muted', fontSize: 2 }}>›</Text>
            </Box>
          ))}
        </Box>
      )}
    </Box>
  );
};
