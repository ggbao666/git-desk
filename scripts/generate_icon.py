"""生成 GitDesk 应用图标：渐变圆角方块 + Git 分支节点图。

设计语言与前端界面一致（紫色 -> 蓝紫渐变），白色分支图代表 Git 提交图：
主干两个提交节点，中间分叉出一个特性分支节点。

运行：python scripts/generate_icon.py
产物：build/windows/icon.ico（Windows 可执行程序图标）
      build/appicon.png    （通用 PNG 图标，512x512）
"""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parent.parent
S = 1024

COLOR_TOP_LEFT = (139, 124, 248)   # #8B7CF8 紫
COLOR_BOTTOM_RIGHT = (91, 110, 245)  # #5B6EF5 蓝紫
WHITE = (255, 255, 255, 255)

# 分支图（1024 坐标系）
TRUNK_X = 360
TRUNK_TOP = 250
TRUNK_BOTTOM = 774
FORK_START = (360, 430)
FORK_CTRL1 = (520, 430)
FORK_CTRL2 = (690, 470)
FORK_END = (690, 610)
NODES = [(360, 250), (360, 774), (690, 610)]
LINE_W = 70
NODE_R = 78


def lerp(a, b, t):
    return tuple(round(x + (y - x) * t) for x, y in zip(a, b))


def bezier(p0, p1, p2, p3, steps=80):
    pts = []
    for i in range(steps + 1):
        t = i / steps
        u = 1 - t
        x = u**3 * p0[0] + 3 * u**2 * t * p1[0] + 3 * u * t**2 * p2[0] + t**3 * p3[0]
        y = u**3 * p0[1] + 3 * u**2 * t * p1[1] + 3 * u * t**2 * p2[1] + t**3 * p3[1]
        pts.append((x, y))
    return pts


def make_master():
    # 对角渐变背景
    grad = Image.new("RGB", (S, S))
    px = grad.load()
    for y in range(S):
        for x in range(S):
            px[x, y] = lerp(COLOR_TOP_LEFT, COLOR_BOTTOM_RIGHT, (x + y) / (2 * S))

    icon = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle((0, 0, S - 1, S - 1), radius=228, fill=255)
    icon.paste(grad, (0, 0), mask)

    # 左上角一层极淡的高光，增加立体感
    glow = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    gd = ImageDraw.Draw(glow)
    gd.ellipse((-420, -680, 760, 240), fill=(255, 255, 255, 26))
    glow = glow.filter(ImageFilter.GaussianBlur(90))
    icon = Image.alpha_composite(icon, glow)
    # 高光不要溢出圆角
    icon.putalpha(Image.composite(icon.getchannel("A"), Image.new("L", (S, S), 0), mask))

    # 用独立的灰度遮罩绘制白色图形，避免 Pillow 折线 joint 产生的条纹伪影
    shape = Image.new("L", (S, S), 0)
    sd = ImageDraw.Draw(shape)
    fork = bezier(FORK_START, FORK_CTRL1, FORK_CTRL2, FORK_END)
    sd.line([(TRUNK_X, TRUNK_TOP), (TRUNK_X, TRUNK_BOTTOM)], fill=255, width=LINE_W)
    sd.line(fork, fill=255, width=LINE_W)
    # 用实心圆盘补齐折线拐点，得到平滑的粗线
    half = LINE_W / 2
    for x, y in fork:
        sd.ellipse((x - half, y - half, x + half, y + half), fill=255)
    for cx, cy in NODES:
        sd.ellipse((cx - NODE_R, cy - NODE_R, cx + NODE_R, cy + NODE_R), fill=255)
    icon.paste(WHITE, (0, 0), shape)
    return icon


def main():
    master = make_master()

    out_ico = ROOT / "build" / "windows" / "icon.ico"
    out_png = ROOT / "build" / "appicon.png"
    out_ico.parent.mkdir(parents=True, exist_ok=True)

    sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    master.save(out_ico, format="ICO", sizes=sizes)
    master.resize((512, 512), Image.LANCZOS).save(out_png, format="PNG")

    print(f"ICO : {out_ico} ({out_ico.stat().st_size} bytes)")
    print(f"PNG : {out_png} ({out_png.stat().st_size} bytes)")
    with Image.open(out_ico) as im:
        print("ICO sizes:", sorted(im.info.get("sizes", [])))


if __name__ == "__main__":
    main()
