import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  OnDestroy,
  effect,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { AisleDefinition, RackCell } from './datacenter.model';

// ── Constants ─────────────────────────────────────────────────────────────────

const TW = 64; // tile width in px
const TH = 32; // tile height in px (TW/2 for 2:1 iso)
const MIN_RACK_H = 20;
const MAX_RACK_H = 80;
const OTHER_H = 14;
const AISLE_GAP = 1.2; // extra row-units of space per aisle
const MARGIN_TOP = MAX_RACK_H + 60; // headroom above first row
const MARGIN_LEFT = 80;

// ── Component ─────────────────────────────────────────────────────────────────

@Component({
  selector: 'app-isometric-canvas',
  template: ` <canvas
    #canvas
    class="block w-full"
    role="img"
    [attr.aria-label]="ariaLabel()"
    (mousemove)="onMouseMove($event)"
    (mouseleave)="onMouseLeave()"
    (click)="onClick($event)"
  >
  </canvas>`,
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export default class IsometricCanvasComponent implements OnDestroy {
  readonly cells = input<RackCell[]>([]);

  readonly aisles = input<AisleDefinition[]>([]);

  readonly ariaLabel = input('Isometric floor plan');

  readonly rackHover = output<string | null>();

  readonly rackClick = output<string>();

  private readonly canvasEl = viewChild<ElementRef<HTMLCanvasElement>>('canvas');

  private readonly hoveredId = signal<string | null>(null);

  private resizeObserver?: ResizeObserver;

  constructor() {
    effect(() => {
      const canvas = this.canvasEl()?.nativeElement;
      if (!canvas) return;

      if (!this.resizeObserver) {
        this.resizeObserver = new ResizeObserver(() => this.redraw());
        this.resizeObserver.observe(canvas.parentElement ?? canvas);
      }

      this.drawScene(canvas, this.cells(), this.aisles(), this.hoveredId());
    });
  }

  ngOnDestroy(): void {
    this.resizeObserver?.disconnect();
  }

  // ── Events ─────────────────────────────────────────────────────────────────

  onMouseMove(event: MouseEvent): void {
    const canvas = this.canvasEl()?.nativeElement;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const hit = this.hitTest(canvas, event.clientX - rect.left, event.clientY - rect.top);
    if (hit !== this.hoveredId()) {
      this.hoveredId.set(hit);
      this.rackHover.emit(hit);
    }
  }

  onMouseLeave(): void {
    this.hoveredId.set(null);
    this.rackHover.emit(null);
  }

  onClick(event: MouseEvent): void {
    const canvas = this.canvasEl()?.nativeElement;
    if (!canvas) return;
    const rect = canvas.getBoundingClientRect();
    const rackId = this.hitTest(canvas, event.clientX - rect.left, event.clientY - rect.top);
    if (rackId) {
      this.rackClick.emit(rackId);
    }
  }

  private redraw(): void {
    const canvas = this.canvasEl()?.nativeElement;
    if (canvas) this.drawScene(canvas, this.cells(), this.aisles(), this.hoveredId());
  }

  // ── Geometry ───────────────────────────────────────────────────────────────

  /** Compute effective row offsets, inserting AISLE_GAP units after rows that have an aisle. */
  private static computeOffsets(rows: string[], aisles: AisleDefinition[]): Map<string, number> {
    const offsets = new Map<string, number>();
    let offset = 0;
    rows.forEach((row) => {
      offsets.set(row, offset);
      offset += aisles.some((a) => a.afterRow === row) ? 1 + AISLE_GAP : 1;
    });
    return offsets;
  }

  private static screen(originX: number, originY: number, col0: number, rowOff: number) {
    return {
      x: originX + (col0 - rowOff) * (TW / 2),
      y: originY + (col0 + rowOff) * (TH / 2),
    };
  }

  private static rackH(cell: RackCell) {
    return cell.ownership === 'other-client'
      ? OTHER_H
      : MIN_RACK_H + (cell.fillPct / 100) * (MAX_RACK_H - MIN_RACK_H);
  }

  // ── Scene ──────────────────────────────────────────────────────────────────

  private drawScene(
    canvas: HTMLCanvasElement,
    cells: RackCell[],
    aisles: AisleDefinition[],
    hoveredId: string | null,
  ): void {
    const rows = [...new Set(cells.map((c) => c.row))].sort();
    const maxCol = Math.max(...cells.map((c) => c.col), 1);
    const offsets = IsometricCanvasComponent.computeOffsets(rows, aisles);
    const maxOff = (offsets.get(rows[rows.length - 1]) ?? 0) + 1;

    const dpr = window.devicePixelRatio || 1;
    const cssW = canvas.parentElement?.clientWidth || canvas.offsetWidth || 600;
    const cssH = MARGIN_TOP + (maxCol + maxOff) * (TH / 2) + MAX_RACK_H + 60;

    const canvasEl = canvas;
    canvasEl.style.width = '100%';
    canvasEl.style.height = `${cssH}px`;
    canvasEl.width = Math.round(cssW * dpr);
    canvasEl.height = Math.round(cssH * dpr);

    const ctx = canvasEl.getContext('2d')!;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, cssW, cssH);

    // Origin: shift right enough that the leftmost point (col0=0, rowOff=maxOff) stays visible
    const originX = MARGIN_LEFT + maxOff * (TW / 2);
    const originY = MARGIN_TOP;
    const S = (col0: number, rowOff: number) =>
      IsometricCanvasComponent.screen(originX, originY, col0, rowOff);

    const cellMap = new Map<string, RackCell>();
    cells.forEach((c) => cellMap.set(`${c.row}-${c.col}`, c));

    // Pass 1 — floor tiles (all rows, back→front, col ascending = col0=0 first)
    rows.forEach((row) => {
      const ro = offsets.get(row)!;
      for (let col = 1; col <= maxCol; col += 1) {
        const cell = cellMap.get(`${row}-${col}`);
        if (!cell) continue; // eslint-disable-line no-continue
        const { x, y } = S(col - 1, ro);
        this.floorTile(ctx, x, y, cell.ownership === 'other-client');
      }
    });

    // Pass 2 — aisle strips in the gap between rows
    for (let ri = 1; ri < rows.length; ri += 1) {
      const aisle = aisles.find((a) => a.afterRow === rows[ri - 1]);
      if (!aisle) continue; // eslint-disable-line no-continue
      const aisleStart = offsets.get(rows[ri - 1])! + 1; // just after the row
      const aisleEnd = offsets.get(rows[ri])!; // just before next row
      for (let col = 1; col <= maxCol; col += 1) {
        const { x: x0, y: y0 } = S(col - 1, aisleStart);
        const { x: x1, y: y1 } = S(col - 1, aisleEnd);
        this.aisleTile(ctx, x0, y0, x1, y1, aisle.type);
      }
    }

    // Pass 3 — border floor tiles (entrance, sides, front) drawn before racks so racks overlap them
    const backOff = offsets.get(rows[0])! - 1;
    const frontOff = maxOff;

    // Entrance row (top)
    for (let col = 1; col <= maxCol; col += 1) {
      const { x, y } = S(col - 1, backOff);
      this.floorTile(ctx, x, y, false);
    }

    // Front row (bottom)
    for (let col = 1; col <= maxCol; col += 1) {
      const { x, y } = S(col - 1, frontOff);
      this.floorTile(ctx, x, y, false);
    }

    // Left and right border columns — one tile per rack row + aisle spans + corners
    [-1, maxCol].forEach((borderCol) => {
      // Corner at entrance
      {
        const { x, y } = S(borderCol, backOff);
        this.floorTile(ctx, x, y, false);
      }
      // Tiles at each rack row
      rows.forEach((row) => {
        const { x, y } = S(borderCol, offsets.get(row)!);
        this.floorTile(ctx, x, y, false);
      });
      // Aisle-coloured spans between rows
      for (let ri = 1; ri < rows.length; ri += 1) {
        const aisle = aisles.find((a) => a.afterRow === rows[ri - 1]);
        if (aisle) {
          const aisleStart = offsets.get(rows[ri - 1])! + 1;
          const aisleEnd = offsets.get(rows[ri])!;
          const { x: x0, y: y0 } = S(borderCol, aisleStart);
          const { x: x1, y: y1 } = S(borderCol, aisleEnd);
          this.aisleTile(ctx, x0, y0, x1, y1, aisle.type);
        }
      }
      // Corner at front
      {
        const { x, y } = S(borderCol, frontOff);
        this.floorTile(ctx, x, y, false);
      }
    });

    // Pass 4 — racks (back→front: rows ascending, within row cols ascending)
    rows.forEach((row) => {
      const ro = offsets.get(row)!;
      for (let col = 1; col <= maxCol; col += 1) {
        const cell = cellMap.get(`${row}-${col}`);
        if (!cell) continue; // eslint-disable-line no-continue
        const { x, y } = S(col - 1, ro);
        this.rack(ctx, x, y, IsometricCanvasComponent.rackH(cell), cell, hoveredId === cell.rackId);
      }
    });

    // Pass 5 — labels (always on top of all geometry)
    for (let ri = 1; ri < rows.length; ri += 1) {
      const aisle = aisles.find((a) => a.afterRow === rows[ri - 1]);
      if (!aisle) continue; // eslint-disable-line no-continue
      const aisleStart = offsets.get(rows[ri - 1])! + 1;
      const aisleEnd = offsets.get(rows[ri])!;
      const midOff = (aisleStart + aisleEnd) / 2;
      const midCol = (maxCol - 1) / 2;
      const { x: lx, y: ly } = S(midCol, midOff);
      ctx.save();
      ctx.font = 'bold 9px ui-sans-serif, sans-serif';
      ctx.fillStyle = aisle.type === 'cold' ? '#0284c7' : '#ea580c';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(aisle.type === 'cold' ? '↔  Cold Aisle' : '↔  Hot Aisle', lx, ly + TH / 4);
      ctx.restore();
    }

    const { x: ex, y: ey } = S((maxCol - 1) / 2, backOff);
    ctx.save();
    ctx.font = '10px ui-sans-serif, sans-serif';
    ctx.fillStyle = '#64748b';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('Entrance', ex, ey + TH / 4);
    ctx.restore();

    rows.forEach((row) => {
      const ro = offsets.get(row)!;
      const { x, y } = S(-2, ro);
      ctx.save();
      ctx.font = 'bold 11px ui-sans-serif, sans-serif';
      ctx.fillStyle = '#94a3b8';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(row, x + TW / 2, y + TH / 4);
      ctx.restore();
    });
  }

  // ── Draw primitives ────────────────────────────────────────────────────────

  private readonly floorTile = (
    ctx: CanvasRenderingContext2D,
    x: number,
    y: number,
    other: boolean,
  ): void => {
    const w2 = TW / 2;
    const d2 = TH / 2;
    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.lineTo(x + w2, y + d2);
    ctx.lineTo(x, y + TH);
    ctx.lineTo(x - w2, y + d2);
    ctx.closePath();
    ctx.fillStyle = other ? '#f1f5f9' : '#f0fdf4';
    ctx.fill();
    ctx.strokeStyle = '#e2e8f0';
    ctx.lineWidth = 0.5;
    ctx.stroke();
  };

  private readonly aisleTile = (
    ctx: CanvasRenderingContext2D,
    x0: number,
    y0: number, // screen pos at aisleStart row
    x1: number,
    y1: number, // screen pos at aisleEnd row (same col)
    type: 'cold' | 'hot',
  ): void => {
    const w2 = TW / 2;
    const d2 = TH / 2;
    // Four corners: top-right, bottom-right, bottom-left, top-left of this tile column
    ctx.beginPath();
    ctx.moveTo(x0 + w2, y0 + d2); // top-right corner of start tile
    ctx.lineTo(x0, y0 + TH); // bottom point of start tile
    ctx.lineTo(x1, y1 + TH); // bottom point of end tile
    ctx.lineTo(x1 + w2, y1 + d2); // top-right corner of end tile
    ctx.closePath();
    ctx.fillStyle = type === 'cold' ? 'rgba(186,230,253,0.8)' : 'rgba(254,215,170,0.8)';
    ctx.fill();
    ctx.strokeStyle = type === 'cold' ? '#7dd3fc' : '#fdba74';
    ctx.lineWidth = 0.5;
    ctx.stroke();

    // Also draw the top-left face of the tile column (the other half)
    ctx.beginPath();
    ctx.moveTo(x0 - w2, y0 + d2);
    ctx.lineTo(x0, y0 + TH);
    ctx.lineTo(x1, y1 + TH);
    ctx.lineTo(x1 - w2, y1 + d2);
    ctx.closePath();
    ctx.fillStyle = type === 'cold' ? 'rgba(166,210,233,0.8)' : 'rgba(244,195,150,0.8)';
    ctx.fill();
    ctx.strokeStyle = type === 'cold' ? '#7dd3fc' : '#fdba74';
    ctx.lineWidth = 0.5;
    ctx.stroke();

    // Top face of the aisle strip
    ctx.beginPath();
    ctx.moveTo(x0, y0);
    ctx.lineTo(x0 + w2, y0 + d2);
    ctx.lineTo(x0, y0 + TH);
    ctx.lineTo(x0 - w2, y0 + d2);
    ctx.closePath();
    ctx.fillStyle = type === 'cold' ? 'rgba(186,230,253,0.9)' : 'rgba(254,215,170,0.9)';
    ctx.fill();
    ctx.strokeStyle = type === 'cold' ? '#7dd3fc' : '#fdba74';
    ctx.lineWidth = 0.5;
    ctx.stroke();
  };

  private readonly rack = (
    ctx: CanvasRenderingContext2D,
    x: number,
    y: number,
    h: number,
    cell: RackCell,
    hovered: boolean,
  ): void => {
    const w2 = TW / 2;
    const d2 = TH / 2;
    const isOther = cell.ownership === 'other-client';
    const isIssue = cell.floorStatus === 'issue';

    let topC: string;
    let leftC: string;
    let rightC: string;
    let strokeC: string;
    if (hovered && !isOther) {
      topC = '#c7d2fe';
      leftC = '#a5b4fc';
      rightC = '#818cf8';
      strokeC = '#6366f1';
    } else if (isOther) {
      topC = '#e2e8f0';
      leftC = '#cbd5e1';
      rightC = '#b8c5d3';
      strokeC = '#94a3b8';
    } else if (isIssue) {
      topC = '#fca5a5';
      leftC = '#f87171';
      rightC = '#ef4444';
      strokeC = '#fca5a5';
    } else {
      topC = '#6ee7b7';
      leftC = '#34d399';
      rightC = '#10b981';
      strokeC = '#6ee7b7';
    }
    const lw = hovered ? 1.5 : 0.75;

    // Top face
    ctx.beginPath();
    ctx.moveTo(x, y - h);
    ctx.lineTo(x + w2, y - h + d2);
    ctx.lineTo(x, y - h + TH);
    ctx.lineTo(x - w2, y - h + d2);
    ctx.closePath();
    ctx.fillStyle = topC;
    ctx.fill();
    ctx.strokeStyle = strokeC;
    ctx.lineWidth = lw;
    ctx.stroke();

    // Left face
    ctx.beginPath();
    ctx.moveTo(x - w2, y - h + d2);
    ctx.lineTo(x, y - h + TH);
    ctx.lineTo(x, y + TH);
    ctx.lineTo(x - w2, y + d2);
    ctx.closePath();
    ctx.fillStyle = leftC;
    ctx.fill();
    ctx.strokeStyle = strokeC;
    ctx.lineWidth = lw;
    ctx.stroke();

    // Right face
    ctx.beginPath();
    ctx.moveTo(x, y - h + TH);
    ctx.lineTo(x + w2, y - h + d2);
    ctx.lineTo(x + w2, y + d2);
    ctx.lineTo(x, y + TH);
    ctx.closePath();
    ctx.fillStyle = rightC;
    ctx.fill();
    ctx.strokeStyle = strokeC;
    ctx.lineWidth = lw;
    ctx.stroke();

    if (isOther) {
      // Hatch lines on left face, clipped to the face polygon
      ctx.save();
      ctx.beginPath();
      ctx.moveTo(x - w2, y - h + d2);
      ctx.lineTo(x, y - h + TH);
      ctx.lineTo(x, y + TH);
      ctx.lineTo(x - w2, y + d2);
      ctx.closePath();
      ctx.clip();
      ctx.globalAlpha = 0.3;
      ctx.strokeStyle = '#475569';
      ctx.lineWidth = 1;
      ctx.beginPath();
      const steps = 6;
      for (let i = 0; i <= steps; i += 1) {
        const t = i / steps;
        // Diagonal lines going top-right to bottom-left inside the face
        const sx = x - w2 + t * w2;
        const sy = y - h + d2 + t * (TH + h - d2);
        ctx.moveTo(sx - w2 * 0.6, sy - d2 * 0.4);
        ctx.lineTo(sx + w2 * 0.6, sy + d2 * 0.4);
      }
      ctx.stroke();
      ctx.restore();
      return;
    }

    // Rack label on top face
    ctx.save();
    ctx.font = 'bold 9px ui-monospace, monospace';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillStyle = isIssue ? '#7f1d1d' : '#064e3b';
    ctx.fillText(cell.rackName.split('-').slice(-1)[0], x, y - h + d2 + 2);
    ctx.restore();

    // Fill% on right face
    if (h > 30) {
      ctx.save();
      ctx.font = '8px ui-sans-serif, sans-serif';
      ctx.fillStyle = isIssue ? '#7f1d1d' : '#064e3b';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(`${cell.fillPct}%`, x + w2 / 2, y - h / 2 + d2);
      ctx.restore();
    }

    if (isIssue) {
      ctx.save();
      ctx.font = '11px sans-serif';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText('⚠', x + w2 - 4, y - h + 6);
      ctx.restore();
    }
  };

  private static wallBlock(ctx: CanvasRenderingContext2D, x: number, y: number, h: number): void {
    const w2 = TW / 2;
    const d2 = TH / 2;
    // Top face
    ctx.beginPath();
    ctx.moveTo(x, y - h);
    ctx.lineTo(x + w2, y - h + d2);
    ctx.lineTo(x, y - h + TH);
    ctx.lineTo(x - w2, y - h + d2);
    ctx.closePath();
    ctx.fillStyle = '#cbd5e1';
    ctx.fill();
    ctx.strokeStyle = '#94a3b8';
    ctx.lineWidth = 0.5;
    ctx.stroke();
    // Left face
    ctx.beginPath();
    ctx.moveTo(x - w2, y - h + d2);
    ctx.lineTo(x, y - h + TH);
    ctx.lineTo(x, y + TH);
    ctx.lineTo(x - w2, y + d2);
    ctx.closePath();
    ctx.fillStyle = '#e2e8f0';
    ctx.fill();
    ctx.strokeStyle = '#94a3b8';
    ctx.lineWidth = 0.5;
    ctx.stroke();
    // Right face
    ctx.beginPath();
    ctx.moveTo(x, y - h + TH);
    ctx.lineTo(x + w2, y - h + d2);
    ctx.lineTo(x + w2, y + d2);
    ctx.lineTo(x, y + TH);
    ctx.closePath();
    ctx.fillStyle = '#d1d5db';
    ctx.fill();
    ctx.strokeStyle = '#94a3b8';
    ctx.lineWidth = 0.5;
    ctx.stroke();
  }

  // ── Hit test ───────────────────────────────────────────────────────────────

  private hitTest(canvas: HTMLCanvasElement, mx: number, my: number): string | null {
    const cells = this.cells();
    const aisles = this.aisles();
    const rows = [...new Set(cells.map((c) => c.row))].sort();
    const maxCol = Math.max(...cells.map((c) => c.col), 1);
    const offsets = IsometricCanvasComponent.computeOffsets(rows, aisles);
    const maxOff = (offsets.get(rows[rows.length - 1]) ?? 0) + 1;

    const originX = MARGIN_LEFT + maxOff * (TW / 2);
    const originY = MARGIN_TOP;
    const S = (col0: number, rowOff: number) =>
      IsometricCanvasComponent.screen(originX, originY, col0, rowOff);

    const cellMap = new Map<string, RackCell>();
    cells.forEach((c) => cellMap.set(`${c.row}-${c.col}`, c));

    // Test front-to-back (reverse of draw order)
    for (let ri = rows.length - 1; ri >= 0; ri -= 1) {
      const ro = offsets.get(rows[ri])!;
      for (let col = maxCol; col >= 1; col -= 1) {
        const cell = cellMap.get(`${rows[ri]}-${col}`);
        if (!cell || cell.ownership !== 'own') continue; // eslint-disable-line no-continue
        const { x, y } = S(col - 1, ro);
        const h = IsometricCanvasComponent.rackH(cell);
        if (mx >= x - TW / 2 && mx <= x + TW / 2 && my >= y - h && my <= y + TH) {
          return cell.rackId ?? null;
        }
      }
    }
    return null;
  }
}
