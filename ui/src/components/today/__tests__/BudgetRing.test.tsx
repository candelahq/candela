import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { BudgetRing } from '@/components/today/BudgetRing';

describe('BudgetRing', () => {
  it('renders percentage text and used label', () => {
    render(<BudgetRing percent={50} spent={50} limit={100} remaining={50} />);
    expect(screen.getByText('50%')).toBeInTheDocument();
    expect(screen.getByText('used')).toBeInTheDocument();
  });

  it('shows spent, limit, and remaining formatted as dollars', () => {
    render(<BudgetRing percent={50} spent={50} limit={100} remaining={50} />);
    expect(screen.getAllByText('$50.00')).toHaveLength(2); // spent and remaining
    expect(screen.getByText('$100.00')).toBeInTheDocument(); // limit
  });

  it('uses warning color at 80%+', () => {
    const { container } = render(<BudgetRing percent={85} spent={85} limit={100} remaining={15} />);
    const pctText = container.querySelector('.today-ring-pct');
    expect(pctText).toHaveStyle({ fill: 'var(--warning)' });
  });

  it('uses error color at 100%+', () => {
    const { container } = render(<BudgetRing percent={110} spent={110} limit={100} remaining={-10} />);
    const pctText = container.querySelector('.today-ring-pct');
    expect(pctText).toHaveStyle({ fill: 'var(--error)' });
  });

  it('uses accent color below 80%', () => {
    const { container } = render(<BudgetRing percent={50} spent={50} limit={100} remaining={50} />);
    const pctText = container.querySelector('.today-ring-pct');
    expect(pctText).toHaveStyle({ fill: 'var(--accent)' });
  });

  it('remaining <= 0 uses error color for "Left" value', () => {
    render(<BudgetRing percent={110} spent={110} limit={100} remaining={-10} />);
    const leftValue = screen.getByText('$0.00'); // max(0, remaining)
    expect(leftValue).toHaveStyle({ color: 'var(--error)' });
  });
});
