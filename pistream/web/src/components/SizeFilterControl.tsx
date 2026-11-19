import React, { useEffect, useRef, useState } from 'react';
import { ChevronDown, SlidersHorizontal } from './Icons';

interface SizeFilterControlProps {
  enabled: boolean;
  minSizeMB: number;
  hiddenCount: number;
  onToggle: (enabled: boolean) => void;
  onMinSizeChange: (mb: number) => void;
}

const PRESETS: { label: string; mb: number }[] = [
  { label: '50 MB', mb: 50 },
  { label: '100 MB', mb: 100 },
  { label: '200 MB', mb: 200 },
  { label: '500 MB', mb: 500 },
  { label: '1 GB', mb: 1024 },
];

export const SizeFilterControl: React.FC<SizeFilterControlProps> = ({
  enabled,
  minSizeMB,
  hiddenCount,
  onToggle,
  onMinSizeChange,
}) => {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(String(minSizeMB));
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setDraft(String(minSizeMB));
  }, [minSizeMB]);

  useEffect(() => {
    if (!open) return;
    const handleOutside = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handleOutside);
    window.addEventListener('keydown', handleKey, true);
    return () => {
      document.removeEventListener('mousedown', handleOutside);
      window.removeEventListener('keydown', handleKey, true);
    };
  }, [open]);

  const handleInputChange = (raw: string) => {
    setDraft(raw);
    if (raw.trim() === '') return;
    const parsed = Number(raw);
    if (Number.isFinite(parsed) && parsed >= 0) {
      onMinSizeChange(Math.round(parsed));
    }
  };

  const handleInputBlur = () => {
    const parsed = Number(draft);
    if (!Number.isFinite(parsed) || parsed < 0 || draft.trim() === '') {
      setDraft(String(minSizeMB));
      return;
    }
    const clamped = Math.round(parsed);
    setDraft(String(clamped));
    onMinSizeChange(clamped);
  };

  const handlePresetClick = (mb: number) => {
    setDraft(String(mb));
    onMinSizeChange(mb);
    if (!enabled) {
      onToggle(true);
    }
  };

  return (
    <div ref={rootRef} className="relative inline-block flex-none">
      <button
        type="button"
        aria-label="檔案大小過濾"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={`h-8 px-2.5 rounded-lg border text-xs font-medium flex items-center gap-2 cinema-focus transition-colors duration-150 ease-cinema ${
          enabled
            ? 'bg-accent/10 hover:bg-accent/15 border-accent/50 text-ink-primary'
            : 'bg-surface/95 hover:bg-raised border-border hover:border-border-strong text-ink-primary'
        }`}
      >
        <SlidersHorizontal
          size={14}
          strokeWidth={1.5}
          className={enabled ? 'text-accent' : 'text-ink-muted'}
        />
        <span className="whitespace-nowrap tabular-nums">
          {enabled ? `過濾 ≥ ${minSizeMB} MB` : '過濾'}
        </span>
        <ChevronDown
          size={14}
          strokeWidth={1.5}
          className={`text-ink-muted flex-none transition-transform duration-150 ease-cinema ${
            open ? 'rotate-180' : ''
          }`}
        />
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="檔案大小過濾設定"
          className="absolute top-full right-0 mt-1.5 z-50 w-64 p-3.5 rounded-lg bg-surface border border-border-strong shadow-xl"
        >
          <div className="flex items-center justify-between gap-3">
            <span className="text-xs font-medium text-ink-primary">
              隱藏小型影片
            </span>
            <button
              type="button"
              role="switch"
              aria-checked={enabled}
              aria-label="啟用檔案大小過濾"
              onClick={() => onToggle(!enabled)}
              className={`relative inline-flex h-5 w-9 flex-none items-center rounded-full border transition-colors duration-150 ease-cinema cinema-focus ${
                enabled
                  ? 'bg-accent border-accent'
                  : 'bg-raised border-border-strong'
              }`}
            >
              <span
                className={`inline-block h-3.5 w-3.5 rounded-full transition-transform duration-150 ease-cinema ${
                  enabled
                    ? 'translate-x-[18px] bg-deepest'
                    : 'translate-x-[2px] bg-ink-secondary'
                }`}
              />
            </button>
          </div>

          <p className="text-[11px] text-ink-muted mt-1 leading-relaxed">
            僅隱藏小於指定大小的影片檔，不含字幕檔
          </p>

          <div className="mt-3 pt-3 border-t border-border">
            <label
              htmlFor="size-filter-mb-input"
              className="block text-[11px] text-ink-secondary mb-1.5"
            >
              隱藏小於此大小的影片
            </label>
            <div className="flex items-center rounded-lg bg-deepest border border-border focus-within:border-accent/70 overflow-hidden transition-colors duration-150">
              <input
                id="size-filter-mb-input"
                type="number"
                inputMode="numeric"
                min={0}
                step={10}
                value={draft}
                onChange={(e) => handleInputChange(e.target.value)}
                onBlur={handleInputBlur}
                className="w-full h-8 px-2.5 bg-transparent text-xs font-mono tabular-nums text-ink-primary focus:outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
              />
              <span className="px-2.5 text-[11px] font-mono text-ink-muted border-l border-border select-none">
                MB
              </span>
            </div>

            <div className="flex flex-wrap gap-1.5 mt-2.5">
              {PRESETS.map((preset) => {
                const isSelected = minSizeMB === preset.mb;
                return (
                  <button
                    key={preset.mb}
                    type="button"
                    onClick={() => handlePresetClick(preset.mb)}
                    className={`h-6 px-2 rounded-md text-[11px] font-mono tabular-nums border transition-colors duration-150 ease-cinema ${
                      isSelected && enabled
                        ? 'bg-accent/15 border-accent/60 text-accent font-medium'
                        : isSelected
                          ? 'bg-raised border-border-strong text-ink-primary'
                          : 'bg-deepest/70 hover:bg-raised border-border text-ink-secondary hover:text-ink-primary'
                    }`}
                  >
                    {preset.label}
                  </button>
                );
              })}
            </div>
          </div>

          {enabled && (
            <div className="mt-3 pt-2.5 border-t border-border flex items-center justify-between text-[11px] text-ink-muted tabular-nums">
              <span>
                {hiddenCount > 0
                  ? `已隱藏 ${hiddenCount} 部小於 ${minSizeMB} MB 的影片`
                  : `目前沒有小於 ${minSizeMB} MB 的影片`}
              </span>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
