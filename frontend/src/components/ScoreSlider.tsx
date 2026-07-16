import { useState, useCallback, useEffect } from 'react';
import { Box, Button, Label, Text } from '@primer/react';
import type { Criterion, DraftComment } from '../lib/types';

interface ScoreSliderProps {
  criterion: Criterion;
  value: number | null;
  comment: string;
  isFirst?: boolean;
  otherComments?: DraftComment[];
  commentingIndicator?: string; // e.g. "✏️ Ally is commenting"
  onChange: (value: number) => void;
  onCommentChange: (comment: string) => void;
  onCommentDelete?: () => void;
  onCommentFocus?: () => void;
  onCommentBlur?: () => void;
  disabled?: boolean;
}

/* ── Custom slider CSS (injected once) ─────────────────────────────── */
const SLIDER_CSS = `
/* Reset browser defaults */
input[type="range"].score-slider {
  -webkit-appearance: none;
  appearance: none;
  width: 100%;
  height: 26px;
  background: transparent;
  cursor: pointer;
  touch-action: none;
  margin: 0;
}
input[type="range"].score-slider:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
/* ── Track ── */
input[type="range"].score-slider::-webkit-slider-runnable-track {
  height: 6px;
  border-radius: 3px;
  background: var(--bgColor-neutral-muted, #d0d7de);
}
input[type="range"].score-slider::-moz-range-track {
  height: 6px;
  border-radius: 3px;
  border: none;
  background: var(--bgColor-neutral-muted, #d0d7de);
}
/* ── Filled portion (Firefox) ── */
input[type="range"].score-slider::-moz-range-progress {
  height: 6px;
  border-radius: 3px;
  background: var(--fgColor-accent, #0969da);
}
input[type="range"].score-slider.score-slider--unset::-moz-range-progress {
  background: var(--bgColor-neutral-muted, #d0d7de);
}
/* ── Thumb ── */
input[type="range"].score-slider::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--fgColor-accent, #0969da);
  border: 2px solid #fff;
  box-shadow: 0 1px 3px rgba(0,0,0,0.2);
  margin-top: -9px;
  transition: transform 0.1s ease;
}
input[type="range"].score-slider::-moz-range-thumb {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--fgColor-accent, #0969da);
  border: 2px solid #fff;
  box-shadow: 0 1px 3px rgba(0,0,0,0.2);
  transition: transform 0.1s ease;
}
input[type="range"].score-slider.score-slider--unset::-webkit-slider-thumb {
  background: var(--fgColor-muted, #656d76);
}
input[type="range"].score-slider.score-slider--unset::-moz-range-thumb {
  background: var(--fgColor-muted, #656d76);
}
/* Active / hover feedback */
input[type="range"].score-slider:not(:disabled)::-webkit-slider-thumb:hover {
  transform: scale(1.15);
}
input[type="range"].score-slider:not(:disabled):active::-webkit-slider-thumb {
  transform: scale(1.25);
}
input[type="range"].score-slider:not(:disabled)::-moz-range-thumb:hover {
  transform: scale(1.15);
}
input[type="range"].score-slider:not(:disabled):active::-moz-range-thumb {
  transform: scale(1.25);
}
`;

let styleInjected = false;
function injectSliderStyles() {
  if (styleInjected) return;
  const style = document.createElement('style');
  style.textContent = SLIDER_CSS;
  document.head.appendChild(style);
  styleInjected = true;
}

export const ScoreSlider = ({
  criterion,
  value,
  comment,
  isFirst = false,
  otherComments = [],
  commentingIndicator,
  onChange,
  onCommentChange,
  onCommentDelete,
  onCommentFocus,
  onCommentBlur,
  disabled,
}: ScoreSliderProps) => {
  const [commentOpen, setCommentOpen] = useState(comment.length > 0);
  const [guideOpen, setGuideOpen] = useState(false);

  // Inject custom slider CSS once
  useEffect(() => { injectSliderStyles(); }, []);

  // Open the comment section when a previously-saved comment arrives async
  useEffect(() => {
    if (comment.length > 0) {
      setCommentOpen(true);
    }
  }, [comment]);

  const isSet = value !== null;
  const rubric = criterion.rubric;
  const displayValue = value ?? criterion.min_value;
  const range = criterion.max_value - criterion.min_value;
  const percentage = range > 0 ? ((displayValue - criterion.min_value) / range) * 100 : 0;
  const activeBand = isSet
    ? criterion.rubric?.bands.find((band) => displayValue >= band.min_value && displayValue <= band.max_value)
    : undefined;
  const hasOwnComment = comment.trim().length > 0;

  const bandTone = (band: NonNullable<Criterion['rubric']>['bands'][number]) => {
    const scoreRange = Math.max(1, criterion.max_value - criterion.min_value);
    const midpoint = (band.min_value + band.max_value) / 2;
    const normalized = (midpoint - criterion.min_value) / scoreRange;

    if (normalized >= 0.8) {
      return { bg: 'success.subtle', border: 'success.muted', fg: 'success.fg' };
    }
    if (normalized >= 0.6) {
      return { bg: 'accent.subtle', border: 'accent.muted', fg: 'accent.fg' };
    }
    if (normalized >= 0.4) {
      return { bg: 'attention.subtle', border: 'attention.muted', fg: 'attention.fg' };
    }
    if (normalized >= 0.2) {
      return { bg: 'severe.subtle', border: 'severe.muted', fg: 'severe.fg' };
    }
    return { bg: 'danger.subtle', border: 'danger.muted', fg: 'danger.fg' };
  };

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

  const handleDelete = useCallback(() => {
    if (onCommentDelete) {
      onCommentDelete();
      setCommentOpen(false);
    }
  }, [onCommentDelete]);

  // All comments to display (other users' comments)
  const allComments = otherComments.filter((c) => c.comment.length > 0);

  // Inline style for the WebKit filled track (can't use pseudo-element in CSS for dynamic %)
  const trackBackground = isSet
    ? `linear-gradient(to right, var(--fgColor-accent, #0969da) ${percentage}%, var(--bgColor-neutral-muted, #d0d7de) ${percentage}%)`
    : 'var(--bgColor-neutral-muted, #d0d7de)';

  return (
    <Box
      pt={isFirst ? 0 : 3}
      borderTopWidth={isFirst ? 0 : 1}
      borderTopStyle="solid"
      borderTopColor={isFirst ? 'transparent' : 'border.muted'}
      borderLeftWidth={hasOwnComment ? 2 : 0}
      borderLeftStyle="solid"
      borderLeftColor={hasOwnComment ? 'accent.emphasis' : 'transparent'}
      pl={hasOwnComment ? 2 : 0}
    >
      {/* Header */}
      <Box display="flex" justifyContent="space-between" alignItems="baseline" mb={1}>
        <Text sx={{ fontWeight: 'bold', fontSize: 2 }}>{criterion.title}</Text>
        <Text
          sx={{
            fontSize: 1,
            fontWeight: 'semibold',
            fontVariantNumeric: 'tabular-nums',
            color: !isSet ? 'fg.subtle' : disabled ? 'fg.muted' : 'fg.muted',
          }}
        >
          {isSet ? displayValue : 'Not scored'}
        </Text>
      </Box>

      {rubric && (
        <Box mb={2}>
          <Box display="flex" alignItems="center" justifyContent="space-between" sx={{ gap: 2, flexWrap: 'wrap' }}>
            {activeBand ? (
              <Label
                size="small"
                sx={{
                  backgroundColor: bandTone(activeBand).bg,
                  borderColor: bandTone(activeBand).border,
                  color: bandTone(activeBand).fg,
                }}
              >
                {activeBand.label} {activeBand.title}
              </Label>
            ) : (
              <Box />
            )}
            <Button
              size="small"
              variant="invisible"
              onClick={() => setGuideOpen((open) => !open)}
              aria-label={guideOpen ? 'Hide scoring guide' : 'View scoring guide'}
              sx={{ color: 'fg.muted', ':hover': { color: 'fg.default' } }}
            >
              {guideOpen ? 'Hide guide' : 'Scoring guide'}
            </Button>
          </Box>

          {guideOpen && (
            <Box mt={2} p={3} bg="canvas.subtle" borderRadius={2} borderWidth={1} borderStyle="solid" borderColor="border.default">
              {rubric.checklist.length > 0 && (
                <Box mb={3}>
                  <Text sx={{ fontSize: 0, fontWeight: 'bold', mb: 2, display: 'block' }}>
                    What to check
                  </Text>
                  <Box as="ul" sx={{ pl: 3, my: 0 }}>
                    {rubric.checklist.map((item) => (
                      <Box as="li" key={item} sx={{ mb: 1 }}>
                        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{item}</Text>
                      </Box>
                    ))}
                  </Box>
                </Box>
              )}

              <Text sx={{ fontSize: 0, fontWeight: 'bold', mb: 2, display: 'block' }}>
                How to score it
              </Text>
              <Box display="flex" flexDirection="column" sx={{ gap: 2 }}>
                {rubric.bands.map((band) => {
                  const isActiveBand = activeBand?.label === band.label;
                  const tone = bandTone(band);
                  return (
                    <Box
                      key={`${band.label}-${band.min_value}-${band.max_value}`}
                      p={2}
                      borderRadius={2}
                      borderWidth={1}
                      borderStyle="solid"
                      borderColor={isActiveBand ? tone.border : 'border.default'}
                      bg={isActiveBand ? tone.bg : 'canvas.default'}
                    >
                      <Box display="flex" alignItems="center" justifyContent="space-between" sx={{ gap: 2, mb: 1, flexWrap: 'wrap' }}>
                        <Text sx={{ fontSize: 1, fontWeight: 'bold' }}>
                          {band.label} {band.title}
                        </Text>
                        <Text sx={{ fontSize: 0, color: 'fg.muted' }}>
                          {band.min_value} to {band.max_value}
                        </Text>
                      </Box>
                      <Box as="ul" sx={{ pl: 3, my: 0 }}>
                        {band.bullets.map((bullet) => (
                          <Box as="li" key={bullet} sx={{ mb: 1 }}>
                            <Text sx={{ fontSize: 0, color: 'fg.muted' }}>{bullet}</Text>
                          </Box>
                        ))}
                      </Box>
                    </Box>
                  );
                })}
              </Box>
            </Box>
          )}
        </Box>
      )}

      {/* Slider */}
      <Box position="relative">
        <input
          type="range"
          className={`score-slider${!isSet ? ' score-slider--unset' : ''}`}
          aria-label={`Score for ${criterion.title}`}
          min={criterion.min_value}
          max={criterion.max_value}
          step={1}
          value={displayValue}
          onChange={handleChange}
          disabled={disabled}
          style={{
            // WebKit doesn't support dynamic values in pseudo-element styles,
            // so we paint the track gradient inline.
            backgroundImage: isSet ? trackBackground : 'none',
            backgroundColor: 'var(--bgColor-neutral-muted, #d0d7de)',
            backgroundSize: '100% 6px',
            backgroundPosition: 'center',
            backgroundRepeat: 'no-repeat',
          }}
        />
      </Box>

      {/* Commenting indicator from other users */}
      {commentingIndicator && (
        <Box mt={1}>
          <Text sx={{ fontSize: 0, color: 'attention.fg', fontStyle: 'italic' }}>
            {commentingIndicator}
          </Text>
        </Box>
      )}

      {/* Other users' comments — stacked labeled bubbles */}
      {allComments.length > 0 && (
        <Box mt={2} display="flex" flexDirection="column" sx={{ gap: 1 }}>
          {allComments.map((c) => (
            <Box
              key={`${c.user_id}-${c.criterion_id}`}
              p={2}
              bg="canvas.subtle"
              borderRadius={2}
              borderWidth={1}
              borderStyle="solid"
              borderColor="border.default"
            >
              <Text sx={{ fontSize: 0 }}>
                <Text sx={{ fontWeight: 'bold', color: 'fg.default' }}>{c.display_name}:</Text>{' '}
                <Text sx={{ color: 'fg.muted', fontStyle: 'italic' }}>{c.comment}</Text>
              </Text>
            </Box>
          ))}
        </Box>
      )}

      {/* Comment toggle + textarea (own comment) */}
      {!disabled && (
        <Box mt={2}>
          {!commentOpen ? (
            hasOwnComment ? (
              <Box
                p={2}
                borderWidth={1}
                borderStyle="solid"
                borderColor="accent.muted"
                borderRadius={2}
                bg="accent.subtle"
                display="flex"
                justifyContent="space-between"
                alignItems="center"
                sx={{ gap: 2, flexWrap: 'wrap' }}
              >
                <Box sx={{ minWidth: 0, flex: '1 1 260px' }}>
                  <Text sx={{ fontSize: 0, fontWeight: 'semibold', color: 'fg.default', display: 'block' }}>
                    Comment added
                  </Text>
                  <Text
                    sx={{
                      fontSize: 0,
                      color: 'fg.muted',
                      display: 'block',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {comment.trim()}
                  </Text>
                </Box>
                <Button
                  size="medium"
                  variant="default"
                  onClick={() => {
                    setCommentOpen(true);
                    onCommentFocus?.();
                  }}
                >
                  Edit note
                </Button>
              </Box>
            ) : (
              <Box
                p={2}
                borderWidth={1}
                borderStyle="dashed"
                borderColor="border.muted"
                borderRadius={2}
                bg="canvas.default"
                display="flex"
                justifyContent="space-between"
                alignItems="center"
                sx={{ gap: 2, flexWrap: 'wrap' }}
              >
                <Text sx={{ fontSize: 0, color: 'fg.muted' }}>Optional note</Text>
                <Button
                  size="medium"
                  variant="primary"
                  onClick={() => {
                    setCommentOpen(true);
                    onCommentFocus?.();
                  }}
                >
                  Add note
                </Button>
              </Box>
            )
          ) : (
            <Box>
              <Box display="flex" alignItems="center" justifyContent="space-between" mb={1}>
                <Text sx={{ fontSize: 0, fontWeight: 'bold', color: 'fg.default' }}>
                  Score note
                </Text>
                {hasOwnComment && <Label size="small">Saved note</Label>}
              </Box>
              <textarea
                value={comment}
                onChange={handleCommentChange}
                onFocus={onCommentFocus}
                onBlur={onCommentBlur}
                placeholder="Add an optional note to explain this score."
                rows={2}
                style={{
                  width: '100%',
                  padding: '8px 12px',
                  borderRadius: '6px',
                  border: '1px solid var(--borderColor-default, #d0d7de)',
                  backgroundColor: 'var(--bgColor-default, #fff)',
                  fontSize: '16px',
                  fontFamily: 'inherit',
                  resize: 'vertical',
                  minHeight: '48px',
                }}
              />
              {onCommentDelete && (
                <Box mt={1} display="flex" justifyContent="flex-end">
                  <Text
                    as="button"
                    onClick={handleDelete}
                    sx={{
                      fontSize: 0,
                      color: 'danger.fg',
                      cursor: 'pointer',
                      background: 'none',
                      border: 'none',
                      padding: '2px 6px',
                      borderRadius: 1,
                      ':hover': { bg: 'danger.subtle' },
                    }}
                  >
                    ✕ Remove note
                  </Text>
                </Box>
              )}
            </Box>
          )}
        </Box>
      )}

      {/* Read-only: show all comments as bubbles when disabled */}
      {disabled && allComments.length === 0 && comment && (
        <Box mt={2} p={2} bg="canvas.subtle" borderRadius={2}>
          <Text sx={{ fontSize: 0, color: 'fg.muted', fontStyle: 'italic' }}>
            💬 {comment}
          </Text>
        </Box>
      )}
    </Box>
  );
};
