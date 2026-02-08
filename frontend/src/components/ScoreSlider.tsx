import { useCallback } from 'react';
import { Box, Text } from '@primer/react';
import type { Criterion } from '../lib/types';

interface ScoreSliderProps {
  criterion: Criterion;
  value: number;
  onChange: (value: number) => void;
  disabled?: boolean;
}

export const ScoreSlider = ({ criterion, value, onChange, disabled }: ScoreSliderProps) => {
  const range = criterion.max_value - criterion.min_value;
  const percentage = range > 0 ? ((value - criterion.min_value) / range) * 100 : 0;

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(parseInt(e.target.value, 10));
    },
    [onChange],
  );

  return (
    <Box>
      {/* Header */}
      <Box display="flex" justifyContent="space-between" alignItems="baseline" mb={1}>
        <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>{criterion.title}</Text>
        <Text
          sx={{
            fontSize: 3,
            fontWeight: 'bold',
            fontVariantNumeric: 'tabular-nums',
            color: disabled ? 'fg.muted' : 'fg.default',
          }}
        >
          {value}
        </Text>
      </Box>

      {criterion.description && (
        <Text sx={{ color: 'fg.muted', fontSize: 0, mb: 2, display: 'block' }}>
          {criterion.description}
        </Text>
      )}

      {/* Slider */}
      <Box position="relative">
        <input
          type="range"
          min={criterion.min_value}
          max={criterion.max_value}
          step={1}
          value={value}
          onChange={handleChange}
          disabled={disabled}
          style={{
            width: '100%',
            height: '48px',
            cursor: disabled ? 'not-allowed' : 'pointer',
            accentColor: 'var(--fgColor-accent, #0969da)',
            opacity: disabled ? 0.5 : 1,
          }}
        />
        {/* Track fill indicator */}
        <Box
          position="absolute"
          bottom={0}
          left={0}
          right={0}
          display="flex"
          justifyContent="space-between"
        >
          <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{criterion.min_value}</Text>
          <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{criterion.max_value}</Text>
        </Box>
      </Box>

      {/* Value indicator bar */}
      <Box
        mt={1}
        height="4px"
        borderRadius={2}
        bg="neutral.muted"
        overflow="hidden"
      >
        <Box
          height="100%"
          borderRadius={2}
          bg={disabled ? 'neutral.emphasis' : 'accent.emphasis'}
          sx={{
            width: `${percentage}%`,
            transition: 'width 0.1s ease-out',
          }}
        />
      </Box>
    </Box>
  );
};
