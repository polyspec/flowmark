{? notice.closed}
<p class="notice closed"><button type="button" class="link" hy-set="notice.closed=false">공지 펼치기</button></p>
{:}
<div class="notice">
<p>{= notice.text}</p>
<button type="button" class="link" hy-set="notice.closed=true">닫기</button>
</div>
{/}
