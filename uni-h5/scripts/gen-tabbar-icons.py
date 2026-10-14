"""生成 TabBar 图标（纯标准库，无需 Pillow）。

用法：
    python scripts/gen-tabbar-icons.py

产出：src/static/tabbar/{home,orders,mine}[-active].png（81x81 RGBA）

实现：在 4 倍超采样网格上按「线段/圆环有符号距离 + 描边宽度」判定覆盖，
再降采样得到带抗锯齿的 alpha 通道；颜色取自设计令牌。
"""

import math
import os
import struct
import zlib

SIZE = 81
SS = 4  # 超采样倍数
STROKE = 0.052  # 单位为归一化坐标（图标 0..1）

COLOR_NORMAL = (139, 148, 166)  # --muted  #8b94a6
COLOR_ACTIVE = (245, 181, 68)  # --gold   #f5b544

OUT_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "src", "static", "tabbar")


def seg_dist(px, py, ax, ay, bx, by):
    vx, vy = bx - ax, by - ay
    wx, wy = px - ax, py - ay
    l2 = vx * vx + vy * vy
    t = 0.0 if l2 == 0 else max(0.0, min(1.0, (wx * vx + wy * vy) / l2))
    return math.hypot(wx - t * vx, wy - t * vy)


def ring_dist(px, py, cx, cy, r, y_max=None):
    if y_max is not None and py > y_max:
        return 1e9
    return abs(math.hypot(px - cx, py - cy) - r)


def icon_home(x, y):
    return (
        min(
            seg_dist(x, y, 0.50, 0.11, 0.08, 0.47),
            seg_dist(x, y, 0.50, 0.11, 0.92, 0.47),
            seg_dist(x, y, 0.19, 0.45, 0.19, 0.89),
            seg_dist(x, y, 0.81, 0.45, 0.81, 0.89),
            seg_dist(x, y, 0.19, 0.89, 0.81, 0.89),
        )
        < STROKE
    )


def icon_orders(x, y):
    return (
        min(
            seg_dist(x, y, 0.22, 0.12, 0.78, 0.12),
            seg_dist(x, y, 0.22, 0.12, 0.22, 0.88),
            seg_dist(x, y, 0.78, 0.12, 0.78, 0.88),
            seg_dist(x, y, 0.22, 0.88, 0.78, 0.88),
            seg_dist(x, y, 0.36, 0.36, 0.64, 0.36),
            seg_dist(x, y, 0.36, 0.50, 0.64, 0.50),
            seg_dist(x, y, 0.36, 0.64, 0.64, 0.64),
        )
        < STROKE
    )


def icon_mine(x, y):
    head = ring_dist(x, y, 0.50, 0.32, 0.17)
    shoulders = ring_dist(x, y, 0.50, 0.92, 0.34, y_max=0.86)
    return min(head, shoulders) < STROKE


ICONS = {"home": icon_home, "orders": icon_orders, "mine": icon_mine}


def write_png(path, rows):
    raw = b"".join(b"\x00" + bytes(r) for r in rows)

    def chunk(tag, data):
        return (
            struct.pack(">I", len(data))
            + tag
            + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
        )

    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", struct.pack(">IIBBBBB", SIZE, SIZE, 8, 6, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(raw, 9))
    png += chunk(b"IEND", b"")
    with open(path, "wb") as f:
        f.write(png)


def render(shape, rgb):
    rows = []
    inv = 1.0 / (SIZE * SS)
    for py in range(SIZE):
        row = bytearray()
        for px in range(SIZE):
            hits = 0
            for sy in range(SS):
                y = (py * SS + sy + 0.5) * inv
                for sx in range(SS):
                    x = (px * SS + sx + 0.5) * inv
                    if shape(x, y):
                        hits += 1
            alpha = round(255 * hits / (SS * SS))
            row += bytes((rgb[0], rgb[1], rgb[2], alpha))
        rows.append(row)
    return rows


def main():
    os.makedirs(OUT_DIR, exist_ok=True)
    for name, shape in ICONS.items():
        write_png(os.path.join(OUT_DIR, f"{name}.png"), render(shape, COLOR_NORMAL))
        write_png(os.path.join(OUT_DIR, f"{name}-active.png"), render(shape, COLOR_ACTIVE))
        print(f"generated {name}.png / {name}-active.png")
    print("out:", os.path.normpath(OUT_DIR))


if __name__ == "__main__":
    main()
