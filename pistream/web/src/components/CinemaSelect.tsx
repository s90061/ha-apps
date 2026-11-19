import React, { useEffect, useRef, useState } from 'react';
import { Check, ChevronDown } from './Icons';

export interface SelectOption<T extends string | number> {
  value: T;
  label: string;
}

interface CinemaSelectProps<T extends string | number> {
  ariaLabel: string;
  value: T;
  options: SelectOption<T>[];
  onChange: (value: T) => void;
  leftIcon?: React.ReactNode;
  maxWidthClass?: string;
  alignMenu?: 'left' | 'right';
}

export function CinemaSelect<T extends string | number>({
  ariaLabel,
  value,
  options,
  onChange,
  leftIcon,
  maxWidthClass = 'max-w-[220px]',
  alignMenu = 'right',
}: CinemaSelectProps<T>) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  const selected = options.find((opt) => opt.value === value) || options[0];

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

  return (
    <div ref={rootRef} className="relative inline-block flex-none">
      <button
        type="button"
        aria-label={ariaLabel}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className={`h-8 px-2.5 rounded-lg bg-surface/95 hover:bg-raised border border-border hover:border-border-strong text-xs font-medium text-ink-primary flex items-center gap-2 cinema-focus transition-colors duration-150 ease-cinema ${maxWidthClass}`}
      >
        {leftIcon && <span className="text-ink-muted flex-none flex items-center">{leftIcon}</span>}
        <span className="truncate">{selected ? selected.label : ''}</span>
        <ChevronDown
          size={14}
          strokeWidth={1.5}
          className={`text-ink-muted flex-none transition-transform duration-150 ease-cinema ${
            open ? 'rotate-180' : ''
          }`}
        />
      </button>

      {open && (
        <ul
          role="listbox"
          aria-label={ariaLabel}
          className={`absolute top-full mt-1.5 z-50 min-w-full w-max max-w-[300px] py-1 rounded-lg bg-surface border border-border-strong shadow-xl overflow-y-auto max-h-64 ${
            alignMenu === 'right' ? 'right-0' : 'left-0'
          }`}
        >
          {options.map((opt) => {
            const isActive = opt.value === value;
            return (
              <li key={String(opt.value)} role="option" aria-selected={isActive}>
                <button
                  type="button"
                  onClick={() => {
                    onChange(opt.value);
                    setOpen(false);
                  }}
                  className={`w-full px-3 py-2 text-left text-xs flex items-center justify-between gap-3 transition-colors duration-150 ease-cinema ${
                    isActive
                      ? 'bg-raised text-ink-primary font-medium'
                      : 'text-ink-secondary hover:bg-raised/70 hover:text-ink-primary'
                  }`}
                >
                  <span className="truncate">{opt.label}</span>
                  {isActive && (
                    <Check size={13} strokeWidth={1.75} className="text-accent flex-none" />
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
