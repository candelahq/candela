import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act, fireEvent } from '@testing-library/react';
import { ToastProvider, useToast } from '@/components/Toast';

function TestConsumer() {
  const { toast } = useToast();
  return (
    <div>
      <button onClick={() => toast('Test Message', 'info')}>Show Info</button>
      <button onClick={() => toast('Success Message', 'success')}>Show Success</button>
      <button onClick={() => toast('Error Message', 'error')}>Show Error</button>
      <button onClick={() => toast('Default Message')}>Show Default</button>
    </div>
  );
}

describe('Toast', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  it('renders children without toasts initially', () => {
    render(
      <ToastProvider>
        <div data-testid="child">Child Content</div>
      </ToastProvider>
    );
    expect(screen.getByTestId('child')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.queryByText('Test Message')).not.toBeInTheDocument();
  });

  it('shows toast with correct message when toast() called', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Info'));
    expect(screen.getByText('Test Message')).toBeInTheDocument();
  });

  it('renders success icon (✓) for success type', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Success'));
    expect(screen.getByText('✓')).toBeInTheDocument();
  });

  it('renders error icon (✕) for error type', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Error'));
    expect(screen.getByText('✕')).toBeInTheDocument();
  });

  it('renders info icon (ℹ) for default/info type', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Default'));
    expect(screen.getByText('ℹ')).toBeInTheDocument();
  });

  it('auto-dismisses after 4000ms', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Info'));
    expect(screen.getByText('Test Message')).toBeInTheDocument();
    
    act(() => {
      vi.advanceTimersByTime(4000);
    });
    
    expect(screen.queryByText('Test Message')).not.toBeInTheDocument();
  });

  it('allows manual dismiss via close button click', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Info'));
    expect(screen.getByText('Test Message')).toBeInTheDocument();
    
    const closeBtn = screen.getByLabelText('Dismiss notification');
    fireEvent.click(closeBtn);
    
    expect(screen.queryByText('Test Message')).not.toBeInTheDocument();
  });

  it('multiple toasts render simultaneously', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Info'));
    fireEvent.click(screen.getByText('Show Success'));
    
    expect(screen.getByText('Test Message')).toBeInTheDocument();
    expect(screen.getByText('Success Message')).toBeInTheDocument();
  });

  it('container has aria-live="polite"', () => {
    render(
      <ToastProvider>
        <TestConsumer />
      </ToastProvider>
    );
    fireEvent.click(screen.getByText('Show Info'));

    const toastMessage = screen.getByText('Test Message');
    const liveRegion = toastMessage.closest('[aria-live]');
    expect(liveRegion).not.toBeNull();
    expect(liveRegion).toHaveAttribute('aria-live', 'polite');
  });
});
