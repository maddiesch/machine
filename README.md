# Machine

A programable virtual machine.

```text
; Anything that follows a `;` is considered a comment.
; Lines must end with a ;

; You can group and pipe the return value from one function to another.
(alert(response-time GTE 600)|recover(LT 500)).page();

; You can assign a variable from any expression that returns a value.
; Variables are considered constants and cannot be changed once set unless you delete if first with a `_delete` call
const warnID = warn(response-time GTE 300);

; You can use a variable by it's name preceded by a `$`
slack(#team-channel $warnID);

; The `_delete` call is special. It's implemented in the machine itself.
; It can be used to un-set a variable. Once a variable is unset it can be reset.
_delete(warnID);

; You can nest multiple function calls.
scale-up(env(app-name) cpu GT f0.8);

; Every value is a string.
; The only exception's from that are variables that begin with f and have a .
scale-down(env(app-name) cpu LTE f0.4);

; true and false are also special cases and are mapped to their boolean value.
enable(scaling env(app-name) true);
```

## Reserved words

- `true`

- `false`

- `const`
