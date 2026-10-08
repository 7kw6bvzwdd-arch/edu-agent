"""Parse-only regression checks; model scripts are never executed on the host."""
import importlib.util
from pathlib import Path

spec = importlib.util.spec_from_file_location(
    'manim_preflight', Path(__file__).resolve().parents[1] / 'backend-go/manim_preflight.py'
)
preflight = importlib.util.module_from_spec(spec)
spec.loader.exec_module(preflight)


def test_collects_multiple_historical_errors():
    errors = preflight.check('''from manim import *
class Lesson(Scene):
    def construct(self):
        self.camera.background = Rectangle()
        Dot(opacity=0.7)
        wires = [Line(r, r+RIGHT) for r in [LEFT, 0, RIGHT]]
        self.wait(0)
        Dot().move_to(ORIGINAL)
''')
    assert len(errors) == 5
    assert all('第' in e and '行' in e for e in errors)


def test_valid_code_and_text_are_not_rewritten():
    assert preflight.check('''from manim import *
class Lesson(Scene):
    def construct(self):
        self.camera.background_color = '#182e29'
        dot = Dot(fill_opacity=0.7).move_to(ORIGIN)
        wires = [Line(r, r+RIGHT) for r in [LEFT, ORIGIN, RIGHT]]
        label = Text('opacity=0.7 self.wait(0) ORIGINAL')
        self.play(FadeIn(dot, label), run_time=1)
        self.wait(20)
''') == []


def test_syntax_and_negative_durations():
    for source in ['return 0', 'class :', 'self.wait(-1)', 'self.play(FadeIn(x), run_time=0)', 'Line(0, RIGHT)', 'MathTex("x")']:
        assert preflight.check(source)


def test_preflight_does_not_execute_input():
    assert preflight.check('raise RuntimeError("MUST NOT EXECUTE")') == []
