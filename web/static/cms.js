(function(){
'use strict';
/* CMS: sidebar drawer (<1024px) and [data-open] buttons that open a <details> panel. */
var app=document.getElementById('app'),btn=document.getElementById('menuBtn'),scrim=document.getElementById('scrim'),side=document.getElementById('side');
function set(open){if(!app)return;app.classList.toggle('open',open);if(scrim)scrim.hidden=!open;if(btn)btn.setAttribute('aria-expanded',String(open));
  if(open){var a=side.querySelector('[aria-current]')||side.querySelector('a');if(a)a.focus()}}
if(btn&&app){btn.addEventListener('click',function(){set(!app.classList.contains('open'))});
  scrim.addEventListener('click',function(){set(false);btn.focus()});
  document.addEventListener('keydown',function(e){if(e.key==='Escape'&&app.classList.contains('open')){set(false);btn.focus()}});
  matchMedia('(min-width:1024px)').addEventListener('change',function(m){if(m.matches)set(false)})}
document.addEventListener('click',function(e){var b=e.target.closest&&e.target.closest('[data-open]');if(!b)return;var d=document.getElementById(b.dataset.open);if(!d)return;
  d.open=!d.open;if(d.open){d.scrollIntoView({block:'nearest'});var f=d.querySelector('input:not([type=hidden]),.cs-btn,select');if(f)f.focus()}});
/* tables that fit get a page-sticky header; wider ones scroll sideways and stay a keyboard stop */
function fit(){Array.prototype.forEach.call(document.querySelectorAll('.tw'),function(t){t.classList.remove('fit');var ok=t.scrollWidth<=t.clientWidth+1;t.classList.toggle('fit',ok);
  if(ok)t.removeAttribute('tabindex');else t.tabIndex=0})}
fit();addEventListener('resize',fit);
})();

/* copy buttons: data-copy="#id" copies that element's text */
document.addEventListener('click',function(e){var b=e.target.closest&&e.target.closest('[data-copy]');if(!b)return;var el=document.querySelector(b.dataset.copy);if(!el)return;
  var t=el.textContent.trim(),done=function(){var o=b.textContent;b.textContent='Đã chép';setTimeout(function(){b.textContent=o},1500)};
  var sel=function(){var r=document.createRange();r.selectNodeContents(el);var s=getSelection();s.removeAllRanges();s.addRange(r)};
  if(navigator.clipboard&&navigator.clipboard.writeText)navigator.clipboard.writeText(t).then(done,sel);else sel()});
