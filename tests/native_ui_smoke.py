"""Exercise the real GTK form with sample data; never touch user files."""
import contextlib
import importlib.util
import io
import json
import pathlib
import time
import sys

source = pathlib.Path(__file__).resolve().parents[1] / "internal/nativeui/panel.py"
spec = importlib.util.spec_from_file_location("native_panel", source)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
panel = module.Panel()
panel.show_all()
panel.receive({"type": "state", "state": {
    "categories": ["Development", "Learning"],
    "entries": [{"time": "10:15:00", "category": "Development", "minutes": 30, "activity": "Sample native entry"}],
    "pending": 1, "connected": False, "syncing": False, "sync_error": "",
    "next_reminder": int(time.time()) + 1800,
}})
assert panel.save.get_sensitive()
assert len(panel.store) == 1
panel.category.set_active(1)
panel.minutes.set_value(45)
panel.activity.get_buffer().set_text("Native GTK form check")
output = io.StringIO()
with contextlib.redirect_stdout(output):
    panel.submit()
event = json.loads(output.getvalue())
assert event == {"action": "save", "category": "Learning", "minutes": 45, "activity": "Native GTK form check"}
assert not panel.save.get_sensitive()
panel.receive({"type": "error", "message": "Simulated save failure"})
buffer = panel.activity.get_buffer()
assert buffer.get_text(buffer.get_start_iter(), buffer.get_end_iter(), True) == "Native GTK form check"
panel.receive({"type": "saved", "message": "Saved sample entry"})
assert buffer.get_text(buffer.get_start_iter(), buffer.get_end_iter(), True) == ""
panel.receive({"type": "show", "view": "today"})
assert panel.stack.get_visible_child_name() == "today"
assert "30m logged" in panel.history_note.get_text()
for _ in range(10):
    while module.Gtk.events_pending():
        module.Gtk.main_iteration_do(False)
    time.sleep(0.02)
assert panel.get_window().get_state() & module.Gdk.WindowState.ABOVE
assert panel.get_allocated_width() <= 400
assert panel.get_allocated_height() <= 420
panel.anchor()
monitor = panel.get_display().get_primary_monitor() or panel.get_display().get_monitor(0)
area = monitor.get_workarea()
position = panel.get_position()
assert abs(position.root_x - (area.x + area.width - panel.get_allocated_width() - 16)) <= 2
assert abs(position.root_y - (area.y + 16)) <= 2
if "--screenshots" in sys.argv:
    for view in ("log", "today"):
        panel.navigate(view)
        if view == "log":
            panel.activity.get_buffer().set_text("Refined the activity tracker interface and reviewed the Google Sheets sync workflow.")
        for _ in range(15):
            while module.Gtk.events_pending():
                module.Gtk.main_iteration_do(False)
            time.sleep(0.02)
        import cairo
        surface = cairo.ImageSurface(cairo.FORMAT_ARGB32, panel.get_allocated_width(), panel.get_allocated_height())
        panel.draw(cairo.Context(surface))
        surface.write_to_png(f"/tmp/metracker-{view}.png")

panel.destroy()
print("GTK native form smoke check passed (sample data only).")
