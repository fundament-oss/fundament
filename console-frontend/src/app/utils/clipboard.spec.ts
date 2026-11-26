import { vi } from 'vitest';
import copyToClipboard from './clipboard';

describe('copyToClipboard', () => {
  const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard');

  afterEach(() => {
    if (original) {
      Object.defineProperty(navigator, 'clipboard', original);
    } else {
      delete (navigator as { clipboard?: Clipboard }).clipboard;
    }
    vi.restoreAllMocks();
  });

  const setClipboard = (value: Partial<Clipboard> | undefined) => {
    Object.defineProperty(navigator, 'clipboard', { value, configurable: true });
  };

  it('writes through the clipboard API when there is one', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });

    await copyToClipboard('019b4000-1000-7000-8000-000000000001');

    expect(writeText).toHaveBeenCalledWith('019b4000-1000-7000-8000-000000000001');
  });

  it('passes on a refusal from the clipboard API', async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error('denied')) });

    await expect(copyToClipboard('text')).rejects.toThrow('denied');
  });

  // Plain HTTP has no clipboard API: a port-forward for debugging.
  describe('without the clipboard API', () => {
    beforeEach(() => setClipboard(undefined));

    it('copies through a textarea and removes it again', async () => {
      let copied = '';
      const execCommand = vi.fn(() => {
        copied = document.querySelector('textarea')?.value ?? '';
        return true;
      });
      Object.defineProperty(document, 'execCommand', { value: execCommand, configurable: true });

      await copyToClipboard('alice@example.com');

      expect(execCommand).toHaveBeenCalledWith('copy');
      expect(copied).toBe('alice@example.com');
      expect(document.querySelector('textarea')).toBeNull();
    });

    it('throws when the browser refuses, so the caller does not claim success', async () => {
      Object.defineProperty(document, 'execCommand', {
        value: vi.fn(() => false),
        configurable: true,
      });

      await expect(copyToClipboard('text')).rejects.toThrow();
      expect(document.querySelector('textarea')).toBeNull();
    });
  });
});
