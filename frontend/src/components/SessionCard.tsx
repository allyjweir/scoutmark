import { Box, Text, Label } from '@primer/react';
import type { Session } from '../lib/types';

interface SessionCardProps {
  session: Session;
  onClick?: () => void;
  disabled?: boolean;
}

const STATUS_LABELS: Record<string, { variant: 'default' | 'primary' | 'secondary' | 'accent' | 'success' | 'attention' | 'severe' | 'danger' | 'done' | 'sponsors'; text: string }> = {
  ACTIVE: { variant: 'success', text: 'Active' },
  UPCOMING: { variant: 'accent', text: 'Upcoming' },
  CLOSED: { variant: 'default', text: 'Closed' },
};

const formatTime = (iso: string): string => {
  const date = new Date(iso);
  return date.toLocaleDateString('en-GB', {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
};

export const SessionCard = ({ session, onClick, disabled }: SessionCardProps) => {
  const statusConfig = STATUS_LABELS[session.status] ?? STATUS_LABELS.CLOSED;

  return (
    <Box
      as={disabled ? 'div' : 'button'}
      onClick={disabled ? undefined : onClick}
      display="flex"
      flexDirection="column"
      width="100%"
      p={3}
      mb={2}
      borderWidth={1}
      borderStyle="solid"
      borderColor="border.default"
      borderRadius={2}
      bg="canvas.default"
      sx={{
        cursor: disabled ? 'default' : 'pointer',
        opacity: disabled ? 0.6 : 1,
        textAlign: 'left',
        ':hover': disabled ? {} : {
          borderColor: 'accent.emphasis',
          bg: 'canvas.subtle',
        },
        transition: 'border-color 0.15s, background-color 0.15s',
      }}
    >
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={1}>
        <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>{session.name}</Text>
        <Label variant={statusConfig.variant}>{statusConfig.text}</Label>
      </Box>

      <Text sx={{ color: 'fg.muted', fontSize: 1, mb: 1 }}>
        {session.event_name}
      </Text>

      <Text sx={{ color: 'fg.subtle', fontSize: 0 }}>
        {formatTime(session.starts_at)} — {formatTime(session.ends_at)}
      </Text>
    </Box>
  );
};
