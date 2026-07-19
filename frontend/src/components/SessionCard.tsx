import { Box, Text, Label } from '@primer/react';
import type { Session } from '../lib/types';

interface SessionCardProps {
  session: Session;
  onClick?: () => void;
  disabled?: boolean;
  recentlyFinalised?: boolean;
  note?: string;
}

const STATUS_LABELS: Record<string, { variant: 'default' | 'primary' | 'secondary' | 'accent' | 'success' | 'attention' | 'severe' | 'danger' | 'done' | 'sponsors'; text: string }> = {
  ACTIVE: { variant: 'success', text: 'Active' },
  UPCOMING: { variant: 'accent', text: 'Upcoming' },
  LOCKED: { variant: 'danger', text: 'Locked' },
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

const formatTimeLeft = (endsAtIso: string): string => {
  const msLeft = new Date(endsAtIso).getTime() - Date.now();
  if (msLeft <= 0) return 'Locking now';

  const totalMinutes = Math.floor(msLeft / 60000);
  const days = Math.floor(totalMinutes / (24 * 60));
  const hours = Math.floor((totalMinutes % (24 * 60)) / 60);
  const minutes = totalMinutes % 60;

  if (days > 0) return `Locks in ${days}d ${hours}h`;
  if (hours > 0) return `Locks in ${hours}h ${minutes}m`;
  return `Locks in ${minutes}m`;
};

export const SessionCard = ({ session, onClick, disabled, recentlyFinalised, note }: SessionCardProps) => {
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
      borderColor={recentlyFinalised ? 'success.emphasis' : 'border.default'}
      borderRadius={2}
      bg={recentlyFinalised ? 'success.subtle' : 'canvas.default'}
      sx={{
        cursor: disabled ? 'default' : 'pointer',
        opacity: disabled && !recentlyFinalised ? 0.6 : 1,
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
        <Box display="flex" sx={{ gap: 1 }}>
          {recentlyFinalised && (
            <Label variant="success">Scored ✓</Label>
          )}
          <Label variant={statusConfig.variant}>{statusConfig.text}</Label>
        </Box>
      </Box>

      <Text sx={{ color: 'fg.muted', fontSize: 1, mb: 1 }}>
        {session.event_name}
      </Text>

      <Text sx={{ color: 'fg.subtle', fontSize: 0 }}>
        {session.status === 'ACTIVE'
          ? `${formatTimeLeft(session.ends_at)} • until ${formatTime(session.ends_at)}`
          : `${formatTime(session.starts_at)} — ${formatTime(session.ends_at)}`}
      </Text>

      {session.status === 'LOCKED' && session.locked_at && (
        <Text sx={{ color: 'fg.muted', fontSize: 0, mt: 1 }}>
          Locked by {session.locked_by_name || 'admin'} at {formatTime(session.locked_at)}
        </Text>
      )}

      {note && (
        <Text sx={{ color: 'accent.fg', fontSize: 0, mt: 1, fontWeight: 'semibold' }}>
          {note}
        </Text>
      )}
    </Box>
  );
};
