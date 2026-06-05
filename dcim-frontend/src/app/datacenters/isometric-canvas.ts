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
import { RackCell } from './datacenter.model';

// ── Constants ─────────────────────────────────────────────────────────────────

const TW = 64; // tile width in px
const TH = 32; // tile height in px (TW/2 for 2:1 iso)
const MIN_RACK_H = 20;
const MAX_RACK_H = 80;
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

      this.drawScene(canvas, this.cells(), this.hoveredId());
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
    if (canvas) this.drawScene(canvas, this.cells(), this.hoveredId());
  }

  // ── Geometry ───────────────────────────────────────────────────────────────

  /** Sequential row offsets (one row-unit per rack row). */
  private static computeOffsets(rows: string[]): Map<string, number> {
    return new Map(rows.map((row, i) => [row, i]));
  }

  private static screen(originX: number, originY: number, col0: number, rowOff: number) {
    return {
      x: originX + (col0 - rowOff) * (TW / 2),
      y: originY + (col0 + rowOff) * (TH / 2),
    };
  }

  private static rackH(cell: RackCell) {
    return MIN_RACK_H + (cell.fillPct / 100) * (MAX_RACK_H - MIN_RACK_H);
  }

  // ── Scene ──────────────────────────────────────────────────────────────────

  private drawScene(
    canvas: HTMLCanvasElement,
    cells: RackCell[],
    hoveredId: string | null,
  ): void {
    const rows = [...new Set(cells.map((c) => c.row))].sort();
    const maxCol = Math.max(...cells.map((c) => c.col), 1);
    const offsets = IsometricCanvasComponent.computeOffsets(rows);
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

    // Pass 1 — floor tiles (rack rows, back→front)
    rows.forEach((row) => {
      const ro = offsets.get(row)!;
      for (let col = 1; col <= maxCol; col += 1) {
        const cell = cellMap.get(`${row}-${col}`);
        if (!cell) continue; // eslint-disable-line no-continue
        const { x, y } = S(col - 1, ro);
        this.floorTile(ctx, x, y);
      }
    });

    // Pass 2 — border floor tiles (entrance, sides, front) drawn before racks so racks overlap them
    const backOff = offsets.get(rows[0])! - 1;
    const frontOff = maxOff;

    // Entrance row (top)
    for (let col = 1; col <= maxCol; col += 1) {
      const { x, y } = S(col - 1, backOff);
      this.floorTile(ctx, x, y);
    }

    // Front row (bottom)
    for (let col = 1; col <= maxCol; col += 1) {
      const { x, y } = S(col - 1, frontOff);
      this.floorTile(ctx, x, y);
    }

    // Left and right border columns — one tile per rack row + corners
    [-1, maxCol].forEach((borderCol) => {
      const { x: cx, y: cy } = S(borderCol, backOff);
      this.floorTile(ctx, cx, cy);
      rows.forEach((row) => {
        const { x, y } = S(borderCol, offsets.get(row)!);
        this.floorTile(ctx, x, y);
      });
      const { x: fx, y: fy } = S(borderCol, frontOff);
      this.floorTile(ctx, fx, fy);
    });

    // Pass 3 — racks (back→front: rows ascending, within row cols ascending)
    rows.forEach((row) => {
      const ro = offsets.get(row)!;
      for (let col = 1; col <= maxCol; col += 1) {
        const cell = cellMap.get(`${row}-${col}`);
        if (!cell) continue; // eslint-disable-line no-continue
        const { x, y } = S(col - 1, ro);
        this.rack(ctx, x, y, IsometricCanvasComponent.rackH(cell), cell, hoveredId === cell.rackId);
      }
    });

    // Pass 4 — labels (always on top of all geometry)
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
  ): void => {
    const w2 = TW / 2;
    const d2 = TH / 2;
    ctx.beginPath();
    ctx.moveTo(x, y);
    ctx.lineTo(x + w2, y + d2);
    ctx.lineTo(x, y + TH);
    ctx.lineTo(x - w2, y + d2);
    ctx.closePath();
    ctx.fillStyle = '#f0fdf4';
    ctx.fill();
    ctx.strokeStyle = '#e2e8f0';
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

    let topC: string;
    let leftC: string;
    let rightC: string;
    let strokeC: string;
    if (hovered) {
      topC = '#c7d2fe';
      leftC = '#a5b4fc';
      rightC = '#818cf8';
      strokeC = '#6366f1';
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

    // Rack label on top face
    ctx.save();
    ctx.font = 'bold 9px ui-monospace, monospace';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillStyle = '#064e3b';
    ctx.fillText(cell.rackName.split('-').slice(-1)[0], x, y - h + d2 + 2);
    ctx.restore();

    // Fill% on right face
    if (h > 30) {
      ctx.save();
      ctx.font = '8px ui-sans-serif, sans-serif';
      ctx.fillStyle = '#064e3b';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(`${cell.fillPct}%`, x + w2 / 2, y - h / 2 + d2);
      ctx.restore();
    }
  };

  // ── Hit test ───────────────────────────────────────────────────────────────

  private hitTest(canvas: HTMLCanvasElement, mx: number, my: number): string | null {
    const cells = this.cells();
    const rows = [...new Set(cells.map((c) => c.row))].sort();
    const maxCol = Math.max(...cells.map((c) => c.col), 1);
    const offsets = IsometricCanvasComponent.computeOffsets(rows);
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
        if (!cell) continue; // eslint-disable-line no-continue
        const { x, y } = S(col - 1, ro);
        const h = IsometricCanvasComponent.rackH(cell);
        if (mx >= x - TW / 2 && mx <= x + TW / 2 && my >= y - h && my <= y + TH) {
          return cell.rackId;
        }
      }
    }
    return null;
  }
}
