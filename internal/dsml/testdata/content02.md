<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">go test ./internal/dsml/ -run 'TestMessageContent01|TestParseDSMLMessage' -v 2>&1 | tail -60</parameter>
<parameter name="summary" string="true">Run DSML message tests</parameter>
<parameter name="timeout" string="false">120</parameter>
</invoke>
</shell>
