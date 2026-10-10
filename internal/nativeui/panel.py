"""Small native GTK3 activity window. JSON pipes connect it to the Go tracker."""
import json
import sys
import threading
import time
import gi

gi.require_version("Gtk", "3.0")
from gi.repository import Gtk, Gdk, GLib

CSS = b"""
window { background-color: #f6f7f4; color: #243b33; }
label { color: #243b33; }
.brand { font-size: 16px; font-weight: bold; }
.subtitle { color: #69786e; font-size: 11px; }
.heading { font-size: 11px; font-weight: bold; }
button { background-image: none; background-color: #ffffff; color: #345345;
 border: 1px solid #d6dfd5; border-radius: 6px; padding: 6px 10px; box-shadow: none; }
button label { color: inherit; }
button:hover { background-color: #e8eee7; }
button:disabled { color: #9ba89e; background-color: #edf1eb; }
button.primary { background-color: #345c48; color: #ffffff; border-color: #345c48; font-weight: bold; }
button.primary:hover { background-color: #264a38; }
button.quiet { background-color: transparent; border-color: transparent; padding: 4px 7px; }
entry, spinbutton, combobox button { background-color: #ffffff; color: #243b33;
 background-image: none; border: 1px solid #d6dfd5; border-radius: 6px; min-height: 24px; box-shadow: none; }
spinbutton button { border: none; border-radius: 0; padding: 3px; }
textview, textview text { background-color: #ffffff; color: #243b33; font-size: 12px; }
.editor { border: 1px solid #d6dfd5; border-radius: 6px; }
.feedback { color: #345c48; font-size: 11px; }
.error { color: #ae542b; }
list, row { background-color: transparent; }
.history-row { border-bottom: 1px solid #dde4dc; padding: 10px 0; }
scrollbar slider { background-color: #c4d0c2; min-width: 5px; min-height: 5px; }
"""


def emit(action, **values):
    print(json.dumps(dict(action=action, **values), ensure_ascii=False), flush=True)


class Panel(Gtk.Window):
    def __init__(self):
        super().__init__(title="Me Tracker")
        self.set_default_size(380, 350)
        self.set_resizable(False)
        self.set_decorated(False)
        self.set_type_hint(Gdk.WindowTypeHint.UTILITY)
        self.set_skip_taskbar_hint(True)
        self.set_keep_above(True)
        self.connect("map", lambda *_: self.anchor())
        self.connect("destroy", self.on_destroy)
        self.next_reminder = 0
        self.categories = []
        self.entries = None
        self.busy = False
        provider = Gtk.CssProvider()
        provider.load_from_data(CSS)
        Gtk.StyleContext.add_provider_for_screen(Gdk.Screen.get_default(), provider, Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
        root = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=12)
        root.set_border_width(16)
        self.add(root)
        header = Gtk.Box(spacing=6)
        header.pack_start(self.label("me tracker", "brand"), True, True, 0)
        self.view_button = Gtk.Button(label="Today")
        self.view_button.get_style_context().add_class("quiet")
        self.view_button.connect("clicked", lambda _: self.navigate("log" if self.stack.get_visible_child_name() == "today" else "today"))
        header.pack_start(self.view_button, False, False, 0)
        close = Gtk.Button(label="×")
        close.set_tooltip_text("Close (Esc)")
        close.get_style_context().add_class("quiet")
        close.connect("clicked", lambda _: self.destroy())
        header.pack_end(close, False, False, 0)
        root.pack_start(header, False, False, 0)
        self.stack = Gtk.Stack()
        self.stack.set_homogeneous(True)
        root.pack_start(self.stack, True, True, 0)
        form = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=10)
        self.stack.add_named(form, "log")
        form.pack_start(self.label("What did you work on?", "heading"), False, False, 0)
        self.activity = Gtk.TextView()
        self.activity.set_wrap_mode(Gtk.WrapMode.WORD_CHAR)
        self.activity.set_left_margin(10)
        self.activity.set_right_margin(10)
        self.activity.set_top_margin(10)
        self.activity.set_bottom_margin(10)
        scroll = Gtk.ScrolledWindow()
        scroll.get_style_context().add_class("editor")
        scroll.set_min_content_height(85)
        scroll.set_policy(Gtk.PolicyType.NEVER, Gtk.PolicyType.AUTOMATIC)
        scroll.add(self.activity)
        form.pack_start(scroll, True, True, 0)
        row = Gtk.Box(spacing=10)
        category_box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=5)
        category_box.pack_start(self.label("Category", "heading"), False, False, 0)
        self.category = Gtk.ComboBoxText()
        category_box.pack_start(self.category, False, False, 0)
        row.pack_start(category_box, True, True, 0)
        duration_box = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=5)
        duration_box.pack_start(self.label("Minutes", "heading"), False, False, 0)
        self.minutes = Gtk.SpinButton.new_with_range(1, 1440, 1)
        self.minutes.set_value(30)
        self.minutes.set_numeric(True)
        self.minutes.set_width_chars(4)
        duration_box.pack_start(self.minutes, False, False, 0)
        row.pack_start(duration_box, False, False, 0)
        form.pack_start(row, False, False, 0)
        actions = Gtk.Box(spacing=8)
        actions.pack_start(self.label("Ctrl + Enter", "subtitle"), True, True, 0)
        self.save = Gtk.Button(label="Save activity")
        self.save.get_style_context().add_class("primary")
        self.save.set_sensitive(False)
        self.save.connect("clicked", self.submit)
        actions.pack_end(self.save, False, False, 0)
        form.pack_start(actions, False, False, 0)
        self.feedback = self.label("", "feedback")
        self.feedback.set_line_wrap(True)
        self.feedback.set_max_width_chars(42)
        form.pack_start(self.feedback, False, False, 0)
        history = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=8)
        self.stack.add_named(history, "today")
        self.history_note = self.label("No entries yet today.", "heading")
        history.pack_start(self.history_note, False, False, 0)
        self.store = Gtk.ListStore(str, str, int, str)
        self.history_list = Gtk.ListBox()
        self.history_list.set_selection_mode(Gtk.SelectionMode.NONE)
        history_scroll = Gtk.ScrolledWindow()
        history_scroll.set_policy(Gtk.PolicyType.NEVER, Gtk.PolicyType.AUTOMATIC)
        history_scroll.add(self.history_list)
        history.pack_start(history_scroll, True, True, 0)
        root.pack_start(Gtk.Separator(), False, False, 0)
        self.status = self.label("Loading…", "subtitle")
        self.status.set_line_wrap(True)
        self.status.set_max_width_chars(42)
        root.pack_start(self.status, False, False, 0)
        self.countdown = self.label("Next reminder in 30:00", "subtitle")
        root.pack_start(self.countdown, False, False, 0)
        self.navigate("log")
        self.connect("key-press-event", self.key)
        self.clock_source = GLib.timeout_add_seconds(1, self.clock)

    def anchor(self):
        display = self.get_display()
        monitor = display.get_primary_monitor() or display.get_monitor(0)
        if monitor:
            area = monitor.get_workarea()
            width, height = self.get_size()
            self.move(max(area.x, area.x + area.width - width - 16), area.y + 16)
        return False

    def on_destroy(self, *_):
        GLib.source_remove(self.clock_source)
        if Gtk.main_level():
            Gtk.main_quit()

    def navigate(self, view):
        self.stack.set_visible_child_name(view)
        self.view_button.set_label("Log activity" if view == "today" else "Today")

    @staticmethod
    def label(text, css_class):
        label = Gtk.Label(label=text, xalign=0)
        label.get_style_context().add_class(css_class)
        return label

    def submit(self, *_):
        if self.busy:
            return
        buffer = self.activity.get_buffer()
        activity = buffer.get_text(buffer.get_start_iter(), buffer.get_end_iter(), True).strip()
        if not activity or len(activity) > 8000:
            self.notice("Enter an activity of up to 8,000 characters.", True)
            return
        category = self.category.get_active_text()
        if not category:
            self.notice("Choose a category first.", True)
            return
        self.minutes.update()
        self.busy = True
        self.save.set_sensitive(False)
        self.save.set_label("Saving…")
        emit("save", category=category, minutes=self.minutes.get_value_as_int(), activity=activity)

    def key(self, _, event):
        if event.keyval == Gdk.KEY_Escape:
            self.destroy()
            return True
        if event.state & Gdk.ModifierType.CONTROL_MASK and event.keyval in (Gdk.KEY_Return, Gdk.KEY_KP_Enter):
            if self.stack.get_visible_child_name() == "log" and self.save.get_sensitive():
                self.submit()
            return True
        return False

    def notice(self, message, error=False):
        self.feedback.set_text(message)
        style = self.feedback.get_style_context()
        style.remove_class("error")
        if error:
            style.add_class("error")

    def receive(self, message):
        kind = message.get("type")
        if kind == "state":
            state = message["state"]
            categories = state["categories"]
            if categories != self.categories:
                previous = self.category.get_active_text()
                self.category.remove_all()
                for category in categories:
                    self.category.append_text(category)
                self.category.set_active(categories.index(previous) if previous in categories else 0)
                self.categories = categories
            self.save.set_sensitive(bool(categories) and not self.busy)
            entries = state["entries"]
            if entries != self.entries:
                self.store.clear()
                for child in self.history_list.get_children():
                    self.history_list.remove(child)
                for entry in reversed(entries):
                    self.store.append((entry["time"][:5], entry["category"], entry["minutes"], entry["activity"]))
                    item = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=10)
                    item.get_style_context().add_class("history-row")
                    meta = Gtk.Box(spacing=16)
                    meta.pack_start(self.label(entry["time"][:5], "heading"), False, False, 0)
                    meta.pack_start(self.label(entry["category"], "subtitle"), True, True, 0)
                    meta.pack_end(self.label(f'{entry["minutes"]} min', "heading"), False, False, 0)
                    item.pack_start(meta, False, False, 0)
                    activity = self.label(entry["activity"], "activity-text")
                    activity.set_line_wrap(True)
                    activity.set_line_wrap_mode(2)
                    activity.set_max_width_chars(38)
                    activity.set_selectable(True)
                    item.pack_start(activity, False, False, 0)
                    self.history_list.add(item)
                self.history_list.show_all()
                self.entries = entries
                total = sum(entry["minutes"] for entry in entries)
                self.history_note.set_text(f"{len(entries)} {'activity' if len(entries) == 1 else 'activities'}  ·  {total // 60}h {total % 60:02}m logged" if entries else "Your first activity will appear here.")
            self.next_reminder = state["next_reminder"]
            pending = state["pending"]
            cloud = "Google not connected"
            if state["connected"]:
                cloud = "Google syncing" if state["syncing"] else "Google up to date" if pending == 0 else f"{pending} pending uploads"
            if state["sync_error"]:
                cloud = "Google needs attention; entries are safe in Excel"
            self.base_status = f"Excel ready  ·  {cloud}"
            self.status.set_tooltip_text(state["sync_error"] or None)
            self.clock()
        elif kind == "show":
            self.navigate("today" if message.get("view") == "today" else "log")
            self.set_keep_above(True)
            self.present()
            self.anchor()
            window = self.get_window()
            if window:
                window.raise_()
            if message.get("view") != "today":
                self.activity.grab_focus()
        elif kind in ("saved", "error"):
            self.busy = False
            self.save.set_label("Save activity")
            self.save.set_sensitive(bool(self.categories))
            if kind == "saved":
                self.activity.get_buffer().set_text("")
            self.notice(message["message"], kind == "error")
        return False

    def clock(self):
        if hasattr(self, "base_status"):
            remaining = max(0, int(self.next_reminder - time.time()))
            self.status.set_text(self.base_status)
            self.countdown.set_text(f"Next reminder in {remaining // 60:02}:{remaining % 60:02}")
        return True


def main():
    panel = Panel()
    panel.show_all()

    def read_messages():
        try:
            for line in sys.stdin:
                GLib.idle_add(panel.receive, json.loads(line))
        except (ValueError, OSError):
            pass
        GLib.idle_add(Gtk.main_quit)

    threading.Thread(target=read_messages, daemon=True).start()
    Gtk.main()


if __name__ == "__main__":
    main()
