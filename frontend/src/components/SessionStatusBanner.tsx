import { Box, Text } from '@primer/react';
import type { Session } from '../lib/types';

interface SessionStatusBannerProps {
  session: Session;
}

const formatDateTime = (iso?: string): string => {
  if (!iso) return 'Unknown';
  const dt = new Date(iso);
  return dt.toLocaleString(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
};

export const SessionStatusBanner = ({ session }: SessionStatusBannerProps) => {
  const common = {
    p: 2,
    borderRadius: 2,
    borderWidth: 1,
    borderStyle: 'solid' as const,
  };

  if (session.status === 'LOCKED') {
    const actor = session.locked_by_name || 'an administrator';
    return (
      <Box sx={{ ...common, bg: 'danger.subtle', borderColor: 'danger.muted' }}>
        <Text sx={{ fontSize: 1, color: 'fg.default' }}>
          Session locked by <Text as="span" sx={{ fontWeight: 'bold' }}>{actor}</Text> at {formatDateTime(session.locked_at)}. No further scoring edits are allowed.
        </Text>
      </Box>
    );
  }

  if (session.status === 'CLOSED') {
    return (
      <Box sx={{ ...common, bg: 'neutral.subtle', borderColor: 'border.muted' }}>
        <Text sx={{ fontSize: 1, color: 'fg.default' }}>
          Session finished at {formatDateTime(session.ends_at)}. Scores are read-only.
        </Text>
      </Box>
    );
  }

  if (session.status === 'UPCOMING') {
    return (
      <Box sx={{ ...common, bg: 'accent.subtle', borderColor: 'accent.muted' }}>
        <Text sx={{ fontSize: 1, color: 'fg.default' }}>
          Session opens at {formatDateTime(session.starts_at)}.
        </Text>
      </Box>
    );
  }

  return (
    <Box sx={{ ...common, bg: 'success.subtle', borderColor: 'success.muted' }}>
      <Text sx={{ fontSize: 1, color: 'fg.default' }}>
        Session is open. Scores deadline: {formatDateTime(session.ends_at)}.
      </Text>
    </Box>
  );
};
