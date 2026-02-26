import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { Box, Heading, TextInput, Button, Flash, Text } from '@primer/react';
import { useAuth } from '../hooks/useAuth';

export const LoginPage = () => {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const { login } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setLoading(true);

    try {
      await login(username, password);
      navigate('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      minHeight="100vh"
      bg="canvas.default"
      p={4}
    >
      <form onSubmit={handleSubmit} style={{ width: '100%', maxWidth: '360px' }}>
      <Box
        width="100%"
        p={4}
        borderWidth={1}
        borderStyle="solid"
        borderColor="border.default"
        borderRadius={2}
        bg="canvas.subtle"
      >
        <Heading sx={{ fontSize: 4, mb: 1, textAlign: 'center' }}>⚜️ Scoutmark</Heading>
        <Text
          as="p"
          sx={{ color: 'fg.muted', textAlign: 'center', mb: 4, fontSize: 1 }}
        >
          Patrol scoring made simple
        </Text>

        {error && (
          <Flash variant="danger" sx={{ mb: 3 }}>
            {error}
          </Flash>
        )}

        <Box mb={3}>
          <TextInput
            aria-label="Username"
            placeholder="Username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            block
            size="large"
            autoFocus
            autoComplete="username"
          />
        </Box>

        <Box mb={3}>
          <TextInput
            aria-label="Password"
            type="password"
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            block
            size="large"
            autoComplete="current-password"
          />
        </Box>

        <Button
          type="submit"
          variant="primary"
          size="large"
          block
          disabled={loading || !username || !password}
        >
          {loading ? 'Signing in…' : 'Sign in'}
        </Button>
      </Box>
      </form>
    </Box>
  );
};
