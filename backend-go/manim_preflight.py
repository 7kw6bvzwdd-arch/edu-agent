"""Cheap compatibility checks, NOT a security sandbox. Only run inside Docker."""
import ast
import sys


def check(source):
    try:
        tree = ast.parse(source)
        compile(tree, '<animation>', 'exec')  # Compile only; never execute here.
    except (SyntaxError, ValueError) as exc:
        return [f'第{getattr(exc, "lineno", 0)}行：Python语法无效']
    errors = []

    def report(node, message):
        errors.append(f'第{node.lineno}行：{message}')

    def number(node):
        if isinstance(node, ast.Constant) and type(node.value) in (int, float):
            return node.value
        if isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.USub):
            value = number(node.operand)
            return -value if value is not None else None
        return None

    shapes = {'Dot', 'Circle', 'Square', 'Rectangle', 'Polygon', 'Line', 'Arrow', 'Triangle', 'VMobject'}
    for node in ast.walk(tree):
        if isinstance(node, ast.Attribute) and isinstance(node.ctx, ast.Store):
            if node.attr == 'background' and isinstance(node.value, ast.Attribute) and node.value.attr == 'camera':
                report(node, '不要覆盖camera.background像素缓冲；颜色请用camera.background_color')
        if isinstance(node, ast.Name) and node.id == 'ORIGINAL' and isinstance(node.ctx, ast.Load):
            report(node, '原点常量应为ORIGIN，不是ORIGINAL')
        if not isinstance(node, ast.Call):
            continue
        name = node.func.id if isinstance(node.func, ast.Name) else ''
        method = node.func.attr if isinstance(node.func, ast.Attribute) else ''
        if name in {'Tex', 'MathTex'}:
            report(node, '请用Text，当前动画契约不使用Tex/MathTex')
        if name in shapes and any(k.arg == 'opacity' for k in node.keywords):
            report(node, '构造函数不接受opacity；请用fill_opacity/stroke_opacity或set_opacity()')
        if method == 'wait':
            duration = node.args[0] if node.args else next((k.value for k in node.keywords if k.arg == 'duration'), None)
            if duration is not None and number(duration) is not None and number(duration) <= 0:
                report(node, 'wait时长必须为正数；不等待时删除该调用')
        if method == 'play':
            for keyword in node.keywords:
                if keyword.arg == 'run_time' and number(keyword.value) is not None and number(keyword.value) <= 0:
                    report(node, 'play的run_time必须为正数')
        if name in {'Line', 'Arrow'}:
            points = node.args[:2] + [k.value for k in node.keywords if k.arg in {'start', 'end'}]
            if any(number(p) is not None for p in points):
                report(node, '端点须用三维坐标，原点用ORIGIN，不能用标量0')
    # Common model bug: Line(r, r+RIGHT) for r in [LEFT, 0, RIGHT].
    for comp in ast.walk(tree):
        if not isinstance(comp, (ast.ListComp, ast.GeneratorExp)):
            continue
        for gen in comp.generators:
            if not isinstance(gen.target, ast.Name) or not isinstance(gen.iter, (ast.List, ast.Tuple)):
                continue
            if not any(number(p) is not None for p in gen.iter.elts):
                continue
            for call in ast.walk(comp.elt):
                if isinstance(call, ast.Call) and isinstance(call.func, ast.Name) and call.func.id in {'Line', 'Arrow'}:
                    if any(isinstance(p, ast.Name) and p.id == gen.target.id for p in call.args[:2]):
                        report(call, '端点列表须全部为三维坐标，标量0请改为ORIGIN')
    return list(dict.fromkeys(errors))[:20]


if __name__ == '__main__':
    with open(sys.argv[1], encoding='utf-8-sig') as source:
        errors = check(source.read())
    if errors:
        print('ANIMATION_PREFLIGHT: ' + '\n'.join(errors), file=sys.stderr)
        sys.exit(2)
