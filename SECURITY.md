# Security Policy

## Supported versions

Only the latest release of MacroRec receives security fixes. Please update before reporting.

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it privately in one of these ways:

1. Use GitHub's **"Report a vulnerability"** button on the repository's **Security** tab (private vulnerability reporting).
2. Or email **[your contact email]**.

Please include:

- What the problem is and what an attacker could do with it
- Steps to reproduce, and a sample macro file if one is involved
- Your Windows version and the MacroRec version

What to expect: an acknowledgement within about 7 days, a fix or a clear answer as soon as it is practical, and credit in the release notes if you want it. Please give a reasonable time to fix the problem before you share it publicly.

## Scope

In scope: the MacroRec application and its macro file handling (recording, playback, saving, loading).

Out of scope: problems in Windows itself, in the games or apps you use macros with, social engineering, and "a macro does something harmful when it is replayed" (see **Macro files** below, which is by design).

## What MacroRec does and does not do

MacroRec uses two standard Windows features: **Raw Input** to watch input while recording and **SendInput** to replay it. It does not open other programs' processes, read or write their memory, inject DLLs, install hooks into other programs, or use the network.

MacroRec has **not had an independent security audit**. The list below is what is known, not a guarantee that nothing else exists.

## Known risks and limitations

| ID | Risk | Impact | Mitigation | Status |
| --- | --- | --- | --- | --- |
| MR-01 | **Macro files can do anything you could do at the keyboard.** A macro is a list of key presses and clicks that is replayed as if you typed it. A malicious file can open the Run box, type commands, change settings or delete files, with your permissions. | Arbitrary actions on your PC | A warning popup on first launch and on every **Open**, with **No** as the default. Files are checked for valid format and value ranges. Only open files you made or got from people you trust. | By design. Cannot be fully fixed. |
| MR-02 | **Recording captures every key you press, in every window,** including passwords, while recording is on. | Secrets can end up in a macro file | Recording only runs after you press F8 or Record. The hotkey itself is never recorded. Stop recording before typing anything private. | By design |
| MR-03 | **Macro files are plain text and unencrypted.** Anything typed while recording can be read from the file, including secrets. | Passwords and other text exposed to anyone who can read the file | Do not record sensitive input. Do not share macro files you recorded near it. | Known limitation |
| MR-04 | **The last recording is saved automatically** to `%AppData%\MacroRec\last-macro.txt` and loaded at startup, with no prompt. | Secrets stay on disk after the session. Malware that can write to your user profile could replace this file, and the next F9 would play its contents. | The file is checked on load like any other. Check what is loaded before you press F9. Do not record sensitive input. | Known limitation |
| MR-05 | **Running as administrator raises the damage.** If MacroRec is run as administrator, macros replay with administrator rights. | A bad macro could change system settings | Only run as administrator if the target app requires it. | Known limitation |
| MR-06 | **The exe is unsigned.** Windows cannot show who published it, and you cannot tell a genuine copy from a tampered one by its signature. | Fake or modified copies are possible | Download only from this repository. Build from source if in doubt. Code signing is planned when a certificate is available. | Known limitation |
| MR-07 | **DLL search order.** The app loads some Windows libraries by name. If a malicious DLL with the same name is placed in the same folder as the exe, it might be loaded. | Code execution as you | Run MacroRec from a folder only you can write to, not a shared or temporary folder. Hardening is planned: load system DLLs from the system directory only. | Open. Not yet confirmed or fixed |
| MR-08 | **Very large macro files** can use a lot of memory or run for a long time. | Slowdown or a long-running macro | Files over 64 MB, over 5 million events, or longer than 24 hours are rejected. Press F9 at any time to stop playback. | Mitigated |
| MR-09 | **The "mouse bump" protection is a convenience, not a security boundary.** It tells your hand from the macro by the device the input came from. Other software that sends input could be mistaken for either. | The macro may not stop when expected | Press F9 to stop playback yourself. Do not rely on it to contain an untrusted macro. | Known limitation |
| MR-10 | **Global hotkeys.** F8 and F9 work while other apps are in front, and other software on your PC can send those keys. | Another program could start or stop playback | Keep your PC free of untrusted software. | Known limitation |
| MR-11 | **Games and online services may ban automated input.** This is a policy risk, not a software flaw. | Account bans | Check the rules of the game or service first. | Not a vulnerability |

## Staying safe

- Only open macro files you made yourself, or that came from people you know and trust.
- Do not record passwords, card numbers or other secrets.
- Download MacroRec only from this repository, or build it from source.
- Run it as a normal user unless the target app needs administrator rights.
- Look at what a macro does with a short test run before you rely on it.

## Fixing and disclosure

When a vulnerability is confirmed, it will be fixed in a new release and described in the release notes. The known-risk table above is kept up to date as items are fixed or added.
