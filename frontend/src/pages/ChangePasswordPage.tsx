import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Box, Button, FormControl, TextInput, Text, Flash, Heading } from '@primer/react';
import { useAuth } from '../hooks/useAuth';
import * as api from '../lib/api';

export const ChangePasswordPage = () => {
  const navigate = useNavigate();
  const { user, clearPasswordChangeRequired } = useAuth();
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  if (!user) {
    navigate('/login', { replace: true });
    return null;
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess('');

    if (!newPassword || !confirmPassword) {
      setError('All fields are required');
      return;
    }

    if (newPassword !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    if (newPassword.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }

    setIsLoading(true);
    try {
      await api.changePassword(newPassword);
      setSuccess('Password changed successfully!');
      setNewPassword('');
      setConfirmPassword('');
      clearPasswordChangeRequired();
      setTimeout(() => {
        navigate('/', { replace: true });
      }, 1500);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to change password');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Box display="flex" justifyContent="center" alignItems="center" minHeight="100vh" padding={4}>
      <Box width={400} display="flex" flexDirection="column">
        <Heading as="h1" sx={{ mb: 3 }}>Change Your Password</Heading>
        <Text as="p" sx={{ mb: 3, color: 'fg.muted' }}>
          You're required to change your password on first login.
        </Text>

        {error && <Flash variant="danger" sx={{ mb: 3 }}>{error}</Flash>}
        {success && <Flash variant="success" sx={{ mb: 3 }}>{success}</Flash>}

        <form onSubmit={handleSubmit}>
          <FormControl sx={{ mb: 3 }}>
            <FormControl.Label>New Password</FormControl.Label>
            <TextInput
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              disabled={isLoading}
              autoFocus
              aria-label="New Password"
            />
            <FormControl.Caption>At least 8 characters</FormControl.Caption>
          </FormControl>

          <FormControl sx={{ mb: 3 }}>
            <FormControl.Label>Confirm Password</FormControl.Label>
            <TextInput
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              disabled={isLoading}
              aria-label="Confirm Password"
            />
          </FormControl>

          <Button
            variant="primary"
            size="large"
            type="submit"
            disabled={isLoading}
            sx={{ width: '100%' }}
          >
            {isLoading ? 'Changing Password...' : 'Change Password'}
          </Button>
        </form>
      </Box>
    </Box>
  );
};
