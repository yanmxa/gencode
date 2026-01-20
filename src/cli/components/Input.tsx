import { Box, Text, useInput } from 'ink';
import TextInput from 'ink-text-input';
import { colors, icons } from './theme.js';

interface PromptInputProps {
  value: string;
  onChange: (value: string) => void;
  onSubmit: (value: string) => void;
  hint?: string;
}

export function PromptInput({ value, onChange, onSubmit, hint }: PromptInputProps) {
  const handleSubmit = (text: string) => {
    if (text.trim()) {
      onSubmit(text);
    }
  };

  // Get terminal full width for border
  const width = process.stdout.columns || 80;
  const border = '─'.repeat(width);

  return (
    <Box flexDirection="column" marginTop={0}>
      <Text color={colors.textMuted}>{border}</Text>
      <Box>
        <Text color={colors.brand}>{icons.prompt} </Text>
        <TextInput
          value={value}
          onChange={onChange}
          onSubmit={handleSubmit}
          placeholder=""
        />
        {hint && <Text dimColor> {hint}</Text>}
      </Box>
      <Text color={colors.textMuted}>{border}</Text>
    </Box>
  );
}

interface ConfirmPromptProps {
  message: string;
  onConfirm: (confirmed: boolean) => void;
}

export function ConfirmPrompt({ message, onConfirm }: ConfirmPromptProps) {
  useInput((input, key) => {
    if (input.toLowerCase() === 'y' || key.return) {
      onConfirm(true);
    } else if (input.toLowerCase() === 'n' || key.escape) {
      onConfirm(false);
    }
  });

  return (
    <Box>
      <Text color={colors.warning}>{icons.warning} </Text>
      <Text>{message} </Text>
      <Text color={colors.textMuted}>[y/n] </Text>
    </Box>
  );
}
