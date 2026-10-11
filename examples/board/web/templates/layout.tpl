<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{# title}</title>
<link rel="stylesheet" href="/assets/app.css">
{? reader}<link rel="stylesheet" href="/assets/reader.css">
{/}<script type="module" src="{= assets.hyper}"></script>
</head>
<body hx-boost:inherited="true" hx-target:inherited="#content" hx-swap:inherited="innerMorph">
<div class="shell">
<aside id="left">{# left}</aside>
<main id="content">{# content}</main>
</div>
{# data}
</body>
</html>
