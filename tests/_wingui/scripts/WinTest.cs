// WinTest reads windows and their MSAA objects of another process for the
// PowerShell scripts in this directory. It sends no global input: messages go
// to a given window only.
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

// IAccessible is declared here up to accHelp, in vtable order, so that the
// file compiles without a reference to the Accessibility assembly (which
// pwsh 7.6 on .NET 10 rejects).
[ComImport, Guid("618736E0-3C3D-11CF-810C-00AA00389B71"), InterfaceType(ComInterfaceType.InterfaceIsDual)]
interface IAccessible
{
	[return: MarshalAs(UnmanagedType.IDispatch)] object get_accParent();
	int get_accChildCount();
	[return: MarshalAs(UnmanagedType.IDispatch)] object get_accChild(object varChild);
	string get_accName(object varChild);
	string get_accValue(object varChild);
	string get_accDescription(object varChild);
	object get_accRole(object varChild);
	object get_accState(object varChild);
	string get_accHelp(object varChild);
}

public static class WinTest
{
	public const uint OBJID_WINDOW = 0;
	public const uint OBJID_CLIENT = 0xFFFFFFFC;

	[DllImport("oleacc.dll")]
	static extern int AccessibleObjectFromWindow(IntPtr hwnd, uint id, ref Guid iid, [MarshalAs(UnmanagedType.Interface)] out object ppv);
	[DllImport("oleacc.dll", CharSet = CharSet.Unicode)]
	static extern uint GetRoleText(uint role, StringBuilder sb, uint cch);
	delegate bool EnumProc(IntPtr h, IntPtr l);
	[DllImport("user32.dll")] static extern bool EnumChildWindows(IntPtr h, EnumProc f, IntPtr l);
	[DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetClassName(IntPtr h, StringBuilder sb, int n);
	[DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern int GetWindowText(IntPtr h, StringBuilder sb, int n);
	[DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
	[DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr h);
	[DllImport("user32.dll")] static extern int GetWindowLong(IntPtr h, int i);
	[DllImport("user32.dll")] static extern IntPtr GetParent(IntPtr h);
	[DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint msg, IntPtr w, IntPtr l);

	[StructLayout(LayoutKind.Sequential)] struct RECT { public int Left, Top, Right, Bottom; }
	[StructLayout(LayoutKind.Sequential)]
	struct GUITHREADINFO { public int cbSize, flags; public IntPtr hwndActive, hwndFocus, hwndCapture, hwndMenuOwner, hwndMoveSize, hwndCaret; public RECT rc; }
	[DllImport("user32.dll")] static extern bool GetGUIThreadInfo(uint tid, ref GUITHREADINFO gti);

	public static string Cls(IntPtr h) { var sb = new StringBuilder(256); GetClassName(h, sb, 256); return sb.ToString(); }
	public static string Txt(IntPtr h) { var sb = new StringBuilder(512); GetWindowText(h, sb, 512); return sb.ToString(); }

	static IntPtr[] Children(IntPtr parent)
	{
		var l = new List<IntPtr>();
		EnumChildWindows(parent, (h, x) => { l.Add(h); return true; }, IntPtr.Zero);
		return l.ToArray();
	}

	static string S(Func<object> f) { try { var o = f(); return o == null ? "" : o.ToString(); } catch (Exception e) { return "<" + e.GetType().Name + ">"; } }

	// Acc describes the MSAA object objid of window h: role, name, value,
	// state, help, description and child count.
	public static string Acc(IntPtr h, uint objid)
	{
		Guid iid = new Guid("618736E0-3C3D-11CF-810C-00AA00389B71"); // IID_IAccessible
		object o;
		int hr = AccessibleObjectFromWindow(h, objid, ref iid, out o);
		if (hr != 0 || o == null) return "AccessibleObjectFromWindow hr=0x" + hr.ToString("x");
		var a = (IAccessible)o;
		string role = S(() => a.get_accRole(0));
		string roleText = role;
		int ri;
		if (int.TryParse(role, out ri)) { var sb = new StringBuilder(128); GetRoleText((uint)ri, sb, 128); roleText = sb.ToString() + "(" + ri + ")"; }
		string state = S(() => "0x" + Convert.ToInt32(a.get_accState(0)).ToString("x"));
		return "role=" + roleText + " name='" + S(() => a.get_accName(0)) + "' value='" + S(() => a.get_accValue(0)) + "' state=" + state + " help='" + S(() => a.get_accHelp(0)) + "' desc='" + S(() => a.get_accDescription(0)) + "' children=" + S(() => a.get_accChildCount());
	}

	// Dump describes the top-level window and every visible child window,
	// indented by depth, with the MSAA client object of each.
	public static string Dump(IntPtr top)
	{
		var sb = new StringBuilder();
		sb.AppendLine("TOP " + Cls(top) + " '" + Txt(top) + "' window: " + Acc(top, OBJID_WINDOW) + " | client: " + Acc(top, OBJID_CLIENT));
		foreach (var c in Children(top))
		{
			if (!IsWindowVisible(c)) continue;
			int depth = 0; for (var p = GetParent(c); p != IntPtr.Zero && p != top; p = GetParent(p)) depth++;
			bool tab = (GetWindowLong(c, -16) & 0x10000) != 0; // WS_TABSTOP
			sb.AppendLine(new string(' ', 2 + depth * 2) + Cls(c) + (tab ? " [TABSTOP]" : "") + ": " + Acc(c, OBJID_CLIENT));
		}
		return sb.ToString();
	}

	// FocusOf returns the focused window of the thread that owns top.
	public static IntPtr FocusOf(IntPtr top)
	{
		uint pid; uint tid = GetWindowThreadProcessId(top, out pid);
		var g = new GUITHREADINFO(); g.cbSize = Marshal.SizeOf(g);
		if (!GetGUIThreadInfo(tid, ref g)) return IntPtr.Zero;
		return g.hwndFocus;
	}

	public static string FocusDesc(IntPtr top)
	{
		var f = FocusOf(top);
		if (f == IntPtr.Zero) return "focus=<none>";
		return "focus=" + Cls(f) + " '" + Txt(f) + "' " + Acc(f, OBJID_CLIENT);
	}

	// Tab posts a Tab key press to window h only.
	public static void Tab(IntPtr h)
	{
		PostMessage(h, 0x100, (IntPtr)9, (IntPtr)0x000F0001); // WM_KEYDOWN VK_TAB
		PostMessage(h, 0x101, (IntPtr)9, (IntPtr)unchecked((int)0xC00F0001)); // WM_KEYUP
	}

	public static void Close(IntPtr h)
	{
		PostMessage(h, 0x10, IntPtr.Zero, IntPtr.Zero); // WM_CLOSE
	}
}
