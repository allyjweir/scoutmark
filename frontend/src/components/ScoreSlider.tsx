import { useState, useCallback } from 'react';
import { Box, Text } from '@primer/react';
import type { Criterion } from '../lib/types';

interface ScoreSliderProps {
  criterion: Criterion;
  value: number | null;
  comment: string;
  onChange: (value: number) => void;
  onCommentChange: (comment: string) => void;
  disabled?: boolean;
}

export const ScoreSlider = ({ criterion, value, comment, onChange, onCommentChange, disabled }: ScoreSliderProps) => {
  const [commentOpen, setCommentOpen] = useState(comment.length > 0);
  const isSet = value !== null;
  const displayValue = value ?? criterion.min_value;
  const range = criterion.max_value - criterion.min_value;
  const percentage = range > 0 ? ((displayValue - criterion.min_value) / range) * 100 : 0;

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      onChange(parseInt(e.target.value, 10));
    },
    [onChange],
  );

  const handleCommentChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      onCommentChange(e.target.value);
    },
    [onCommentChange],
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
            color: !isSet ? 'fg.subtle' : disabled ? 'fg.muted' : 'fg.default',
          }}
        >
          {isSet ? displayValue : '–'}
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
          value={displayValue}
          onChange={handleChange}
          disabled={disabled}
          style={{
            width: '100%',
            height: '48px',
            cursor: disabled ? 'not-allowed' : 'pointer',
            accentColor: isSet
              ? 'var(--fgColor-accent, #0969da)'
              : 'var(--fgColor-muted, #656d76)',
            opacity: disabled ? 0.5 : isSet ? 1 : 0.4,
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
          bg={!isSet ? 'neutral.muted' : disabled ? 'neutral.emphasis' : 'accent.emphasis'}
          sx={{
            width: `${percentage}%`,
            transition: 'width 0.1s ease-out',
          }}
        />
      </Box>

      {/* Comment toggle + textarea */}
      {!disabled && (
        <Box mt={2}>
          {!commentOpen ? (
            <Text
              as="button"
              onClick={() => setCommentOpen(true)}
              sx={{
                fontSize: 0,
                color: 'fg.muted',
                cursor: 'pointer',
                background: 'none',
                border: 'none',
                padding: 0,
                textDecoration: 'underline',
                textDecorationStyle: 'dotted',
                ':hover': { color: 'fg.default' },
              }}
            >
              + Add comment
            </Text>
          ) : (
            <textarea
              value={comment}
              onChange={handleCommentChange}
              placeholder="Optional comment…"
              rows={2}
              style={{
                width: '100%',
                padding: '8px 12px',
                borderRadius: '6px',
                border: '1px solid var(--borderColor-default, #d0d7de)',
                backgroundColor: 'var(--bgColor-default, #fff)',
                fontSize: '13px',
                fontFamily: 'inherit',
                resize: 'vertical',
                minHeight: '48px',
              }}
            />
          )}
        </Box>
      )}

      {/* Read-only comment display */}
      {disabled && comment && (
        <Box mt={2} p={2} bg="canvas.subtle" borderRadius={2}>
          <Text sx={{ fontSize: 0, color: 'fg.muted', fontStyle: 'italic' }}>
            💬 {comment}
          </Text>
        </Box>
      )}
    </Box>
  );
};
