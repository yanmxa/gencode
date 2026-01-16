/**
 * QuestionPrompt Component - Claude Code style structured questioning UI
 *
 * Matches Claude Code's AskUserQuestion UI pattern:
 * - Progress bar with step indicators (← □ Step1 □ Step2 ✓ Submit →)
 * - Numbered options with cursor indicator
 * - Description on separate line
 * - Keyboard hints at bottom
 */

import { useState, useCallback } from 'react';
import { Box, Text, useInput } from 'ink';
import { colors } from './theme.js';
import type { Question, QuestionAnswer } from '../../tools/types.js';

// ============================================================================
// Types
// ============================================================================

interface QuestionPromptProps {
  questions: Question[];
  onComplete: (answers: QuestionAnswer[]) => void;
  onCancel?: () => void;
}

interface OptionWithOther {
  label: string;
  description: string;
  isOther?: boolean;
}

// ============================================================================
// Progress Bar Component
// ============================================================================

interface ProgressBarProps {
  questions: Question[];
  currentIndex: number;
  answers: QuestionAnswer[];
  showSubmit?: boolean;
}

function ProgressBar({ questions, currentIndex, answers, showSubmit = true }: ProgressBarProps) {
  const isReviewMode = currentIndex >= questions.length;

  return (
    <Box>
      <Text color={colors.textMuted}>← </Text>
      {questions.map((q, idx) => {
        const isCompleted = idx < answers.length;
        const isCurrent = idx === currentIndex;
        const checkmark = isCompleted ? '☒' : '☐';

        return (
          <Box key={idx}>
            {isCurrent ? (
              <Text backgroundColor="#6366F1" color="#FFFFFF" bold>
                {` □ ${q.header} `}
              </Text>
            ) : (
              <Text color={isCompleted ? colors.textSecondary : colors.textMuted}>
                {checkmark} {q.header}
              </Text>
            )}
            <Text color={colors.textMuted}> </Text>
          </Box>
        );
      })}
      {showSubmit && (
        <>
          {isReviewMode ? (
            <Text backgroundColor="#22C55E" color="#FFFFFF" bold>
              {` ✓ Submit `}
            </Text>
          ) : (
            <Text color={colors.textMuted}>✓ Submit</Text>
          )}
        </>
      )}
      <Text color={colors.textMuted}> →</Text>
    </Box>
  );
}

// ============================================================================
// QuestionPrompt Component
// ============================================================================

export function QuestionPrompt({ questions, onComplete, onCancel }: QuestionPromptProps) {
  const [currentQuestionIndex, setCurrentQuestionIndex] = useState(0);
  const [answers, setAnswers] = useState<QuestionAnswer[]>([]);
  const [selectedOptions, setSelectedOptions] = useState<Set<string>>(new Set());
  const [optionIndex, setOptionIndex] = useState(0);
  const [showOtherInput, setShowOtherInput] = useState(false);
  const [otherInput, setOtherInput] = useState('');
  const [isReviewMode, setIsReviewMode] = useState(false);
  const [reviewOptionIndex, setReviewOptionIndex] = useState(0);

  const currentQuestion = questions[currentQuestionIndex];
  const showProgressBar = questions.length > 1;

  // Add "Type something." option to the list
  const optionsWithOther: OptionWithOther[] = currentQuestion
    ? [
        ...currentQuestion.options,
        { label: 'Type something.', description: '', isOther: true },
      ]
    : [];

  // Toggle option selection (for multi-select)
  const toggleOption = useCallback(() => {
    const option = optionsWithOther[optionIndex];
    setSelectedOptions((prev) => {
      const next = new Set(prev);
      if (next.has(option.label)) {
        next.delete(option.label);
      } else {
        next.add(option.label);
      }
      return next;
    });
  }, [optionIndex, optionsWithOther]);

  // Finish current question and move to next or review
  const finishQuestion = useCallback(
    (selected: string[], customInput?: string) => {
      const answer: QuestionAnswer = {
        question: currentQuestion.question,
        header: currentQuestion.header,
        selectedOptions: selected.filter((s) => s !== 'Type something.'),
        customInput,
      };

      const newAnswers = [...answers, answer];

      if (currentQuestionIndex < questions.length - 1) {
        // Move to next question
        setAnswers(newAnswers);
        setCurrentQuestionIndex((i) => i + 1);
        setSelectedOptions(new Set());
        setOptionIndex(0);
        setShowOtherInput(false);
        setOtherInput('');
      } else {
        // All questions answered, go to review mode (for multi-question)
        setAnswers(newAnswers);
        if (questions.length > 1) {
          setIsReviewMode(true);
          setReviewOptionIndex(0);
        } else {
          // Single question, complete immediately
          onComplete(newAnswers);
        }
      }
    },
    [currentQuestion, currentQuestionIndex, questions.length, answers, onComplete]
  );

  // Handle selection (Enter key)
  const handleSelect = useCallback(() => {
    const option = optionsWithOther[optionIndex];

    if (currentQuestion.multiSelect) {
      if (selectedOptions.size === 0) {
        toggleOption();
        return;
      }
      if (selectedOptions.has('Type something.')) {
        setShowOtherInput(true);
      } else {
        finishQuestion([...selectedOptions]);
      }
    } else {
      if (option.isOther) {
        setShowOtherInput(true);
      } else {
        finishQuestion([option.label]);
      }
    }
  }, [
    optionIndex,
    optionsWithOther,
    currentQuestion?.multiSelect,
    selectedOptions,
    toggleOption,
    finishQuestion,
  ]);

  // Handle keyboard input
  useInput((input, key) => {
    // Review mode navigation
    if (isReviewMode) {
      if (key.upArrow) {
        setReviewOptionIndex((i) => Math.max(0, i - 1));
      } else if (key.downArrow) {
        setReviewOptionIndex((i) => Math.min(1, i + 1));
      } else if (key.return) {
        if (reviewOptionIndex === 0) {
          // Submit
          onComplete(answers);
        } else {
          // Cancel
          onCancel?.();
        }
      } else if (key.escape) {
        onCancel?.();
      }
      return;
    }

    if (showOtherInput) {
      if (key.return) {
        if (otherInput.trim()) {
          if (currentQuestion.multiSelect) {
            const selected = [...selectedOptions].filter((s) => s !== 'Type something.');
            finishQuestion(selected, otherInput.trim());
          } else {
            finishQuestion([], otherInput.trim());
          }
        }
      } else if (key.escape) {
        setShowOtherInput(false);
        setOtherInput('');
      } else if (key.backspace || key.delete) {
        setOtherInput((prev) => prev.slice(0, -1));
      } else if (input && !key.ctrl && !key.meta) {
        setOtherInput((prev) => prev + input);
      }
      return;
    }

    // Navigation
    if (key.upArrow) {
      setOptionIndex((i) => Math.max(0, i - 1));
    } else if (key.downArrow) {
      setOptionIndex((i) => Math.min(optionsWithOther.length - 1, i + 1));
    } else if (key.tab) {
      // Tab cycles through options
      setOptionIndex((i) => (i + 1) % optionsWithOther.length);
    }

    // Selection
    if (key.return) {
      handleSelect();
    }

    // Toggle (multi-select only)
    if (input === ' ' && currentQuestion.multiSelect) {
      toggleOption();
    }

    // Number shortcuts (1-5)
    const num = parseInt(input, 10);
    if (num >= 1 && num <= optionsWithOther.length) {
      setOptionIndex(num - 1);
      if (!currentQuestion.multiSelect) {
        const option = optionsWithOther[num - 1];
        if (option.isOther) {
          setShowOtherInput(true);
        } else {
          finishQuestion([option.label]);
        }
      }
    }

    // Escape to cancel
    if (key.escape && onCancel) {
      onCancel();
    }
  });

  // Review mode UI
  if (isReviewMode) {
    return (
      <Box flexDirection="column" marginTop={1}>
        {/* Progress bar */}
        {showProgressBar && (
          <Box marginBottom={1}>
            <ProgressBar
              questions={questions}
              currentIndex={questions.length}
              answers={answers}
            />
          </Box>
        )}

        {/* Review title */}
        <Box marginBottom={1}>
          <Text bold>Review your answers</Text>
        </Box>

        {/* Answered questions summary */}
        <Box flexDirection="column" marginBottom={1} paddingLeft={1}>
          {answers.map((answer, idx) => {
            const selections = answer.customInput
              ? [...answer.selectedOptions, answer.customInput].join(', ')
              : answer.selectedOptions.join(', ');

            return (
              <Box key={idx} flexDirection="column" marginBottom={1}>
                <Box>
                  <Text color={colors.textSecondary}>● </Text>
                  <Text bold>{answer.question}</Text>
                </Box>
                <Box paddingLeft={2}>
                  <Text color={colors.primary}>→ {selections}</Text>
                </Box>
              </Box>
            );
          })}
        </Box>

        {/* Ready to submit */}
        <Box marginBottom={1}>
          <Text>Ready to submit your answers?</Text>
        </Box>

        {/* Submit/Cancel options */}
        <Box flexDirection="column" paddingLeft={1}>
          <Box>
            <Text color={reviewOptionIndex === 0 ? colors.text : colors.textMuted}>
              {reviewOptionIndex === 0 ? '❯ ' : '  '}
            </Text>
            <Text color={colors.primary} bold={reviewOptionIndex === 0}>
              1. Submit answers
            </Text>
          </Box>
          <Box>
            <Text color={reviewOptionIndex === 1 ? colors.text : colors.textMuted}>
              {reviewOptionIndex === 1 ? '❯ ' : '  '}
            </Text>
            <Text bold={reviewOptionIndex === 1}>2. Cancel</Text>
          </Box>
        </Box>

        {/* Separator */}
        <Box marginTop={1} marginBottom={1}>
          <Text color={colors.textMuted}>{'─'.repeat(60)}</Text>
        </Box>

        {/* Keyboard hints */}
        <Box>
          <Text color={colors.textMuted}>
            Enter to select · Tab/Arrow keys to navigate · Esc to cancel
          </Text>
        </Box>
      </Box>
    );
  }

  // Normal question mode UI
  return (
    <Box flexDirection="column" marginTop={1}>
      {/* Progress bar */}
      {showProgressBar && (
        <Box marginBottom={1}>
          <ProgressBar
            questions={questions}
            currentIndex={currentQuestionIndex}
            answers={answers}
          />
        </Box>
      )}

      {/* Question text */}
      <Box marginBottom={1}>
        <Text bold>{currentQuestion.question}</Text>
      </Box>

      {/* Options list */}
      <Box flexDirection="column" paddingLeft={1}>
        {optionsWithOther.map((option, index) => {
          const isSelected = index === optionIndex;
          const isChecked = selectedOptions.has(option.label);

          return (
            <Box key={option.label} flexDirection="column" marginBottom={option.description ? 1 : 0}>
              {/* Option row */}
              <Box>
                <Text color={isSelected ? colors.text : colors.textMuted}>
                  {isSelected ? '❯ ' : '  '}
                </Text>
                <Text color={colors.textMuted}>{index + 1}. </Text>
                {currentQuestion.multiSelect && (
                  <Text color={colors.primary}>{isChecked ? '☒ ' : '☐ '}</Text>
                )}
                <Text color={colors.primary} bold>
                  {option.label}
                </Text>
              </Box>

              {/* Description (if exists) */}
              {option.description && (
                <Box paddingLeft={4}>
                  <Text color={colors.textMuted}>{option.description}</Text>
                </Box>
              )}
            </Box>
          );
        })}
      </Box>

      {/* Other input field */}
      {showOtherInput && (
        <Box marginTop={1} paddingLeft={1}>
          <Text color={colors.textMuted}>Type your answer: </Text>
          <Text>{otherInput}</Text>
          <Text color={colors.primary}>▋</Text>
        </Box>
      )}

      {/* Separator */}
      <Box marginTop={1} marginBottom={1}>
        <Text color={colors.textMuted}>{'─'.repeat(60)}</Text>
      </Box>

      {/* Chat about this (placeholder) */}
      <Box marginBottom={1}>
        <Text color={colors.textMuted}> Chat about this</Text>
      </Box>

      {/* Keyboard hints */}
      <Box>
        <Text color={colors.textMuted}>
          Enter to select · Tab/Arrow keys to navigate
          {currentQuestion.multiSelect && ' · Space to toggle'}
          {onCancel && ' · Esc to cancel'}
        </Text>
      </Box>
    </Box>
  );
}

// ============================================================================
// Answer Display Component (shown after completion)
// ============================================================================

interface AnswerDisplayProps {
  answers: QuestionAnswer[];
  compact?: boolean;
}

export function AnswerDisplay({ answers, compact = true }: AnswerDisplayProps) {
  return (
    <Box flexDirection="column">
      {answers.map((answer, index) => {
        const selections = answer.customInput
          ? [...answer.selectedOptions, answer.customInput].join(', ')
          : answer.selectedOptions.join(', ');

        return (
          <Box key={index}>
            <Text color={colors.textSecondary}>● </Text>
            <Text>{answer.question}</Text>
            <Text color={colors.textMuted}> → </Text>
            <Text color={colors.primary}>{selections || '(none)'}</Text>
          </Box>
        );
      })}
    </Box>
  );
}
