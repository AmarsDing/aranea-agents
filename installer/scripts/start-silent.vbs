' Silent launcher wrapper — no CMD window at all.
Option Explicit
Dim sh, fso, dir, exe
Set sh = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")
dir = fso.GetParentFolderName(WScript.ScriptFullName)
exe = dir & "\AraneaLauncher.exe"
If Not fso.FileExists(exe) Then
  MsgBox "缺少 AraneaLauncher.exe，请重新安装 Aranea-Agents", vbCritical, "Aranea-Agents"
  WScript.Quit 1
End If
sh.CurrentDirectory = dir
' 0 = hidden window for the launcher process itself (it is already windowsgui)
sh.Run """" & exe & """", 1, False
