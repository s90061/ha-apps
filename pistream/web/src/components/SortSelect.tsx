import React from 'react';
import { ArrowUpDown } from './Icons';
import { SortOption } from '../types';
import { CinemaSelect, SelectOption } from './CinemaSelect';

interface SortSelectProps {
  value: SortOption;
  onChange: (value: SortOption) => void;
}

const SORT_OPTIONS: SelectOption<SortOption>[] = [
  { value: 'date_desc', label: '新增日期：由新到舊' },
  { value: 'date_asc', label: '新增日期：由舊到新' },
  { value: 'name_asc', label: '檔名：A–Z' },
  { value: 'name_desc', label: '檔名：Z–A' },
  { value: 'size_desc', label: '檔案大小：由大到小' },
];

export const SortSelect: React.FC<SortSelectProps> = ({ value, onChange }) => (
  <CinemaSelect
    ariaLabel="排序方式"
    value={value}
    options={SORT_OPTIONS}
    onChange={onChange}
    leftIcon={<ArrowUpDown size={14} strokeWidth={1.5} />}
    maxWidthClass="max-w-[200px]"
    alignMenu="right"
  />
);
