import React from 'react';

export type SubtitleInlineNode =
  | { type: 'text'; text: string }
  | { type: 'br' }
  | { type: 'italic' | 'bold' | 'underline' | 'ruby' | 'rt'; children: SubtitleInlineNode[] };

export interface ActiveSubtitleCue {
  id: string;
  isTopPinned: boolean;
  nodes: SubtitleInlineNode[];
}

function walkDomNodes(domNodes: NodeListOf<ChildNode> | ChildNode[]): SubtitleInlineNode[] {
  const result: SubtitleInlineNode[] = [];
  for (let i = 0; i < domNodes.length; i++) {
    const node = domNodes[i];
    if (node.nodeType === Node.TEXT_NODE) {
      const text = node.textContent || '';
      if (text.length > 0) {
        result.push({ type: 'text', text });
      }
      continue;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) {
      continue;
    }
    const el = node as Element;
    const tag = el.tagName.toLowerCase();
    if (tag === 'br') {
      result.push({ type: 'br' });
      continue;
    }
    const children = walkDomNodes(el.childNodes);
    if (children.length === 0) continue;

    if (tag === 'i' || tag === 'em') {
      result.push({ type: 'italic', children });
    } else if (tag === 'b' || tag === 'strong') {
      result.push({ type: 'bold', children });
    } else if (tag === 'u') {
      result.push({ type: 'underline', children });
    } else if (tag === 'ruby') {
      result.push({ type: 'ruby', children });
    } else if (tag === 'rt') {
      result.push({ type: 'rt', children });
    } else {
      // 安全展開其他 WebVTT 標籤（如 <c> / <v> / <lang>），僅保留文字與允許的格式
      result.push(...children);
    }
  }
  return result;
}

function isCueTopPinned(cue: VTTCue): boolean {
  return (
    typeof cue.line === 'number' &&
    ((cue.snapToLines && cue.line >= 0 && cue.line < 5) || (!cue.snapToLines && cue.line < 50))
  );
}

export function buildActiveCuesSignature(activeCues: TextTrackCueList | null | undefined): string {
  if (!activeCues || activeCues.length === 0) return '';
  const parts: string[] = [];
  for (let i = 0; i < activeCues.length; i++) {
    const cue = activeCues[i] as VTTCue;
    parts.push(`${cue.startTime}:${cue.endTime}:${String(cue.line ?? '')}:${cue.text ?? ''}`);
  }
  return parts.join('\u001f');
}

export function extractActiveSubtitleCues(activeCues: TextTrackCueList | null | undefined): ActiveSubtitleCue[] {
  if (!activeCues || activeCues.length === 0) return [];
  const list: ActiveSubtitleCue[] = [];
  for (let i = 0; i < activeCues.length; i++) {
    const cue = activeCues[i] as VTTCue;
    let nodes: SubtitleInlineNode[] = [];
    try {
      if (typeof cue.getCueAsHTML === 'function') {
        const frag = cue.getCueAsHTML();
        nodes = walkDomNodes(frag.childNodes);
      }
    } catch {
      nodes = [];
    }
    if (nodes.length === 0 && typeof cue.text === 'string' && cue.text.length > 0) {
      nodes = [{ type: 'text', text: cue.text }];
    }
    if (nodes.length === 0) continue;
    list.push({
      id: `${i}_${cue.startTime}_${cue.endTime}`,
      isTopPinned: isCueTopPinned(cue),
      nodes,
    });
  }
  return list;
}

function renderInlineNodes(nodes: SubtitleInlineNode[], keyPrefix: string): React.ReactNode[] {
  return nodes.map((node, idx) => {
    const key = `${keyPrefix}_${idx}`;
    switch (node.type) {
      case 'text':
        return <React.Fragment key={key}>{node.text}</React.Fragment>;
      case 'br':
        return <br key={key} />;
      case 'italic':
        return (
          <em key={key} className="italic">
            {renderInlineNodes(node.children, key)}
          </em>
        );
      case 'bold':
        return (
          <strong key={key} className="font-semibold">
            {renderInlineNodes(node.children, key)}
          </strong>
        );
      case 'underline':
        return (
          <span key={key} className="underline underline-offset-2">
            {renderInlineNodes(node.children, key)}
          </span>
        );
      case 'ruby':
        return <ruby key={key}>{renderInlineNodes(node.children, key)}</ruby>;
      case 'rt':
        return <rt key={key}>{renderInlineNodes(node.children, key)}</rt>;
    }
  });
}

interface SubtitleOverlayProps {
  cues: ActiveSubtitleCue[];
  subtitleScale: number;
  bottomOffsetPx: number;
  topOffsetPx: number;
}

export const SubtitleOverlay: React.FC<SubtitleOverlayProps> = ({
  cues,
  subtitleScale,
  bottomOffsetPx,
  topOffsetPx,
}) => {
  if (cues.length === 0) return null;

  const topCues = cues.filter((c) => c.isTopPinned);
  const bottomCues = cues.filter((c) => !c.isTopPinned);
  const fontSizeRem = `${((1.05 * subtitleScale) / 100).toFixed(2)}rem`;
  const bottomLiftDelta = Math.max(0, bottomOffsetPx - 24);
  const topShiftDelta = Math.max(0, topOffsetPx - 24);

  return (
    <div
      className="pistream-subtitle-overlay absolute inset-0 z-10 pointer-events-none select-none overflow-hidden"
      style={{ fontSize: fontSizeRem }}
      aria-live="off"
    >
      {topCues.length > 0 && (
        <div
          data-testid="subtitle-region-top"
          style={{ transform: `translateY(${topShiftDelta}px)` }}
          className="absolute top-6 inset-x-0 px-4 sm:px-8 flex flex-col items-center gap-1.5 text-center transition-transform duration-200 ease-cinema"
        >
          {topCues.map((cue) => (
            <div key={cue.id} className="max-w-[92%] sm:max-w-[85%]">
              <span className="pistream-subtitle-cue">{renderInlineNodes(cue.nodes, cue.id)}</span>
            </div>
          ))}
        </div>
      )}

      {bottomCues.length > 0 && (
        <div
          data-testid="subtitle-region-bottom"
          style={{ transform: `translateY(-${bottomLiftDelta}px)` }}
          className="absolute bottom-6 inset-x-0 px-4 sm:px-8 flex flex-col items-center gap-1.5 text-center transition-transform duration-200 ease-cinema"
        >
          {bottomCues.map((cue) => (
            <div key={cue.id} className="max-w-[92%] sm:max-w-[85%]">
              <span className="pistream-subtitle-cue">{renderInlineNodes(cue.nodes, cue.id)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
