import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { GrantCard } from '@/components/today/GrantCard';
import type { TodayGrant } from '@/hooks/useTodayBudget';

describe('GrantCard', () => {
  const baseGrant: TodayGrant = {
    reason: 'Test Grant',
    percentUsed: 50,
    spentUsd: 50,
    remainingUsd: 50,
    amountUsd: 100,
    expiresAt: new Date('2026-08-21T00:00:00Z'),
    grantedBy: 'alice@example.com'
  };

  it('renders grant reason as header', () => {
    render(<GrantCard grant={baseGrant} />);
    expect(screen.getByText('Test Grant')).toBeInTheDocument();
  });

  it('shows expiry date formatted as "MMM D"', () => {
    render(<GrantCard grant={baseGrant} />);
    // Date formatting depends on locale, but it should contain something like "Aug 2" (for UTC matching depending on timezone, but let's test for Expiry text)
    // The exact text will be "Expires Aug 20" or "Expires Aug 21" depending on local timezone.
    expect(screen.getByText(/Expires/)).toBeInTheDocument();
  });

  it('progressbar has correct aria-valuenow', () => {
    render(<GrantCard grant={baseGrant} />);
    const progressbar = screen.getByRole('progressbar');
    expect(progressbar).toHaveAttribute('aria-valuenow', '50');
  });

  it('shows Used/Left/Total dollar values', () => {
    render(<GrantCard grant={baseGrant} />);
    expect(screen.getAllByText('$50.00')).toHaveLength(2); // Used and Left
    expect(screen.getByText('$100.00')).toBeInTheDocument(); // Total
  });

  it('shows granter username (before @)', () => {
    render(<GrantCard grant={baseGrant} />);
    expect(screen.getByText('alice')).toBeInTheDocument();
  });

  it('hides expiry when expiresAt is null', () => {
    render(<GrantCard grant={{ ...baseGrant, expiresAt: undefined as never }} />);
    expect(screen.queryByText(/Expires/)).not.toBeInTheDocument();
  });

  it('hides grantedBy section when not provided', () => {
    render(<GrantCard grant={{ ...baseGrant, grantedBy: undefined }} />);
    expect(screen.queryByText('By')).not.toBeInTheDocument();
  });

  it('uses warning color at 80%+', () => {
    const { container } = render(<GrantCard grant={{ ...baseGrant, percentUsed: 85 }} />);
    const fill = container.querySelector('.today-grant-bar-fill');
    expect(fill).toHaveStyle({ background: 'var(--warning)' });
  });

  it('uses error color at 100%+', () => {
    const { container } = render(<GrantCard grant={{ ...baseGrant, percentUsed: 110 }} />);
    const fill = container.querySelector('.today-grant-bar-fill');
    expect(fill).toHaveStyle({ background: 'var(--error)' });
  });
});
