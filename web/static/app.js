(function(){
'use strict';
var $=function(s,r){return (r||document).querySelector(s)},$$=function(s,r){return Array.prototype.slice.call((r||document).querySelectorAll(s))};
var root=document.documentElement,reduce=matchMedia('(prefers-reduced-motion: reduce)').matches;

/* theme */
var mq=matchMedia('(prefers-color-scheme: dark)'),tg=$('#tgl');
function isDark(){return root.dataset.theme?root.dataset.theme==='dark':mq.matches}
function syncTheme(){if(!tg)return;var d=isDark();tg.setAttribute('aria-pressed',String(d));tg.title=d?'Chuyển sang giao diện sáng':'Chuyển sang giao diện tối'}
if(tg){tg.addEventListener('click',function(){root.dataset.theme=isDark()?'light':'dark';try{localStorage.setItem('3c-theme',root.dataset.theme)}catch(e){}syncTheme()});
  syncTheme();if(mq.addEventListener)mq.addEventListener('change',syncTheme)}

/* mobile menu: Esc, outside click and link click close it */
var mn=$('details.mnav');
if(mn){var shut=function(f){if(mn.open){mn.removeAttribute('open');if(f)$('summary',mn).focus()}};
  document.addEventListener('keydown',function(e){if(e.key==='Escape')shut(true)});
  document.addEventListener('click',function(e){if(!mn.contains(e.target))shut();else if(e.target.closest('.mnav-panel a'))shut()})}

/* CMS tabs (mobile row): bring the current tab into view without scrolling the page */
var an=$('nav.anav'),cur=an&&$('[aria-current]',an);
if(cur&&innerWidth<1024)an.scrollLeft=cur.offsetLeft-an.offsetLeft-(an.clientWidth-cur.offsetWidth)/2;

/* live matching */
var form=$('#pf'),res=$('#results');
if(form&&res){
  var timer,ctrl;
  var money=function(m){m=+m;return m>=1000?(m/1000).toLocaleString('vi-VN')+' tỷ':m+' triệu'};
  var labels=function(){$('#oAmount').textContent=money($('#fAmount').value);$('#oTerm').textContent=$('#fTerm').value+' tháng'};
  var qs=function(){return new URLSearchParams(new FormData(form)).toString()};
  var refresh=function(){clearTimeout(timer);timer=setTimeout(function(){
    if(ctrl)ctrl.abort();ctrl=window.AbortController?new AbortController():null;var q=qs();
    fetch('/api/match?'+q,{signal:ctrl&&ctrl.signal,credentials:'same-origin'}).then(function(r){if(!r.ok)throw new Error(r.status);return r.text()})
      .then(function(h){res.innerHTML=h;history.replaceState(null,'','/?'+q+'#tim');syncCmp()}).catch(function(){});
  },250)};
  form.addEventListener('input',function(){labels();refresh()});
  form.addEventListener('change',refresh);
  $$('[data-need]',form).forEach(function(b){b.addEventListener('click',function(){
    $$('input[name=need]',form).forEach(function(i){i.checked=i.value===b.dataset.need});
    $$('[data-need]',form).forEach(function(x){x.setAttribute('aria-pressed',String(x===b))});refresh()})});
}

/* compare tray */
var cmp=[];
function tray(){var t=$('#tray');if(!t)return;t.hidden=cmp.length===0;$('#trayTxt').textContent='Đã chọn '+cmp.length+'/3';$('#trayGo').disabled=cmp.length<2}
function syncCmp(){$$('[data-cmp]').forEach(function(c){c.checked=cmp.indexOf(c.dataset.cmp)>=0});tray()}
document.addEventListener('change',function(e){var c=e.target.closest&&e.target.closest('[data-cmp]');if(!c)return;var id=c.dataset.cmp,i=cmp.indexOf(id);
  if(c.checked&&i<0){if(cmp.length>=3){c.checked=false;return}cmp.push(id)}if(!c.checked&&i>=0)cmp.splice(i,1);tray()});
var tg2=$('#trayGo');if(tg2)tg2.addEventListener('click',function(){var q=form?new URLSearchParams(new FormData(form)):new URLSearchParams();q.set('ids',cmp.join(','));location.href='/so-sanh?'+q.toString()});
var tc=$('#trayClr');if(tc)tc.addEventListener('click',function(){cmp=[];syncCmp()});

/* reveal */
if(!reduce&&'IntersectionObserver' in window){var io=new IntersectionObserver(function(en){en.forEach(function(x){if(x.isIntersecting){x.target.classList.remove('pre');io.unobserve(x.target)}})},{threshold:.15});
  $$('.reveal').forEach(function(el){if(el.getBoundingClientRect().top>innerHeight){el.classList.add('pre');io.observe(el)}})}

/* other-need links in the empty state: switch the live form instead of reloading */
document.addEventListener('click',function(e){var a=e.target.closest&&e.target.closest('[data-pick-need]');if(!a||!form)return;
  var b=$('[data-need="'+a.dataset.pickNeed+'"]',form);if(!b)return;e.preventDefault();b.click();$('#tim').scrollIntoView()});

/* scroll strips are keyboard stops only when they actually scroll */
function stops(){$$('.logos.hscroll,.posts.hscroll').forEach(function(el){if(el.scrollWidth>el.clientWidth+1){el.tabIndex=0;el.setAttribute('role','region')}else{el.removeAttribute('tabindex');el.removeAttribute('role')}})}
stops();addEventListener('resize',stops);

/* custom select: the native <select> stays (hidden) and keeps submitting; a button + listbox is drawn on top */
var fold=function(t){return t.normalize('NFD').replace(/[̀-ͯ]/g,'').replace(/[đĐ]/g,'d').toLowerCase()};
var pop=document.createElement('div'),scrim=document.createElement('div'),cs=null,act=-1,typed='',typedAt=0,seq=0;
pop.className='cs-pop';pop.hidden=true;scrim.className='cs-scrim';scrim.hidden=true;
pop.innerHTML='<input class="cs-q" type="text" autocomplete="off" spellcheck="false" placeholder="Gõ để tìm, ví dụ: ho chi minh" aria-label="Tìm trong danh sách"><ul class="cs-list" role="listbox" id="cs-list"></ul><p class="cs-none" hidden>Không có kết quả</p>';
var cq=$('.cs-q',pop),ul=$('.cs-list',pop),none=$('.cs-none',pop);
function items(){return $$('li:not([hidden])',ul)}
function mark(i){var L=items();if(!L.length)return;act=Math.max(0,Math.min(i,L.length-1));$$('li.act',ul).forEach(function(li){li.classList.remove('act')});
  var li=L[act];li.classList.add('act');(cs.q?cq:cs.b).setAttribute('aria-activedescendant',li.id);
  var t=li.offsetTop,h=li.offsetHeight;if(t<ul.scrollTop)ul.scrollTop=t;else if(t+h>ul.scrollTop+ul.clientHeight)ul.scrollTop=t+h-ul.clientHeight}
function place(){var r=cs.b.getBoundingClientRect(),sheet=innerWidth<=600;pop.classList.toggle('sheet',sheet);scrim.hidden=!sheet;
  if(sheet){pop.style.cssText='';return}
  var w=Math.min(Math.max(r.width,220),innerWidth-16),below=innerHeight-r.bottom-12,up=below<240&&r.top>below;
  pop.style.cssText='left:'+Math.max(8,Math.min(r.left,innerWidth-w-8))+'px;width:'+w+'px;'+(up?'bottom:'+(innerHeight-r.top+6)+'px;max-height:'+Math.min(380,r.top-12)+'px':'top:'+(r.bottom+6)+'px;max-height:'+Math.min(380,Math.max(below,160))+'px')}
function filter(){var f=fold(cq.value.trim()),n=0;$$('li',ul).forEach(function(li){var ok=!f||li.dataset.k.indexOf(f)>=0;li.hidden=!ok;if(ok)n++});none.hidden=n>0;mark(0)}
function open(c){if(cs)close();cs=c;var s=c.s;ul.innerHTML='';
  Array.prototype.forEach.call(s.options,function(o,i){if(o.hidden)return;var li=document.createElement('li');li.id='cs-o'+i;li.setAttribute('role','option');li.dataset.i=i;li.dataset.k=fold(o.text);
    li.textContent=o.text;li.setAttribute('aria-selected',String(i===s.selectedIndex));if(o.disabled)li.setAttribute('aria-disabled','true');ul.appendChild(li)});
  c.q=s.options.length>10;cq.hidden=!c.q;cq.value='';none.hidden=true;
  var lb=c.b.getAttribute('aria-labelledby');if(lb){ul.setAttribute('aria-labelledby',lb.split(' ')[0]);ul.removeAttribute('aria-label')}else{ul.removeAttribute('aria-labelledby');ul.setAttribute('aria-label',c.b.getAttribute('aria-label')||'')}
  document.body.appendChild(scrim);document.body.appendChild(pop);pop.hidden=false;c.b.setAttribute('aria-expanded','true');place();
  var L=items();mark(Math.max(0,L.indexOf($('[aria-selected="true"]',ul))));if(c.q){cq.setAttribute('aria-controls','cs-list');cq.focus()}}
function close(focus){if(!cs)return;var b=cs.b;pop.hidden=true;scrim.hidden=true;b.setAttribute('aria-expanded','false');b.removeAttribute('aria-activedescendant');cs=null;if(focus)b.focus()}
function choose(i){var s=cs.s,o=s.options[i];if(!o||o.disabled)return;close(true);if(s.selectedIndex!==i){s.selectedIndex=i;s.dispatchEvent(new Event('change',{bubbles:true}))}}
function jump(c,ch){var now=Date.now();typed=(now-typedAt<700?typed:'')+fold(ch);typedAt=now;var step=typed.length>1?0:1,k,x;
  if(cs){var L=items();for(k=0;k<L.length;k++){x=L[(act+step+k)%L.length];if(x.dataset.k.indexOf(typed)===0){mark(L.indexOf(x));return}}return}
  var o=c.s.options;for(k=0;k<o.length;k++){x=(c.s.selectedIndex+step+k)%o.length;if(!o[x].hidden&&!o[x].disabled&&fold(o[x].text).indexOf(typed)===0){c.s.selectedIndex=x;c.s.dispatchEvent(new Event('change',{bubbles:true}));return}}}
function keys(e){var c=cs||this.cs,k=e.key,ch=k.length===1&&!e.ctrlKey&&!e.metaKey&&!e.altKey;if(!c)return;
  if(!cs){if(k==='ArrowDown'||k==='ArrowUp'||k==='Enter'||k===' '||k==='F4'){e.preventDefault();open(c)}else if(ch){e.preventDefault();jump(c,k)}return}
  if(k==='ArrowDown'||k==='ArrowUp'){e.preventDefault();mark(act+(k==='ArrowDown'?1:-1))}
  else if(k==='PageDown'||k==='PageUp'){e.preventDefault();mark(act+(k==='PageDown'?8:-8))}
  else if((k==='Home'||k==='End')&&e.target!==cq){e.preventDefault();mark(k==='Home'?0:1e9)}
  else if(k==='Enter'||(k===' '&&e.target!==cq)){e.preventDefault();var li=items()[act];if(li)choose(+li.dataset.i)}
  else if(k==='Escape'){e.preventDefault();e.stopPropagation();close(true)}
  else if(k==='Tab')close();
  else if(ch&&e.target!==cq){e.preventDefault();jump(c,k)}}
cq.addEventListener('input',filter);cq.addEventListener('keydown',keys);
ul.addEventListener('click',function(e){var li=e.target.closest('li');if(li)choose(+li.dataset.i)});
ul.addEventListener('mousemove',function(e){var li=e.target.closest('li');if(li&&!li.classList.contains('act'))mark(items().indexOf(li))});
scrim.addEventListener('click',function(){close(true)});
document.addEventListener('mousedown',function(e){if(cs&&!pop.contains(e.target)&&!cs.b.contains(e.target))close()});
addEventListener('resize',function(){if(cs)place()});
addEventListener('scroll',function(e){if(cs&&!pop.contains(e.target)&&innerWidth>600)close()},true);
function enhance(s){if(s.multiple||s.size>1||s.dataset.cs)return;s.dataset.cs='1';var id='cs'+(++seq);
  var w=document.createElement('span'),b=document.createElement('button'),v=document.createElement('span');w.className='cs';b.type='button';b.className='cs-btn';b.id=id;v.className='cs-v';b.appendChild(v);
  b.setAttribute('role','combobox');b.setAttribute('aria-haspopup','listbox');b.setAttribute('aria-expanded','false');b.setAttribute('aria-controls','cs-list');
  var lab=s.id&&$('label[for="'+s.id+'"]');if(lab){lab.id=lab.id||id+'-l';lab.htmlFor=id;b.setAttribute('aria-labelledby',lab.id+' '+id)}else b.setAttribute('aria-label',s.getAttribute('aria-label')||s.name);
  s.parentNode.insertBefore(w,s);w.appendChild(s);w.appendChild(b);s.classList.add('cs-native');s.tabIndex=-1;s.setAttribute('aria-hidden','true');
  var c={s:s,b:b};b.cs=c;var sync=function(){var o=s.options[s.selectedIndex];v.textContent=o?o.text:''};sync();
  s.addEventListener('change',sync);if(s.form)s.form.addEventListener('reset',function(){setTimeout(sync)});
  b.addEventListener('click',function(){if(cs===c)close(true);else open(c)});b.addEventListener('keydown',keys)}
$$('select').forEach(enhance);

/* hero particles */
var cv=$('#fx');if(!cv)return;var cx=cv.getContext('2d'),W=0,H=0,Q=[],mx=-999,my=-999;
function size(){var d=Math.min(2,devicePixelRatio||1),r=cv.getBoundingClientRect();W=r.width;H=r.height;cv.width=W*d;cv.height=H*d;cx.setTransform(d,0,0,d,0,0);var n=Math.round(Math.min(80,W*H/15000));Q=[];for(var i=0;i<n;i++)Q.push({x:Math.random()*W,y:Math.random()*H,vx:(Math.random()-.5)*.25,vy:(Math.random()-.5)*.25,r:.6+Math.random()*1.5,c:i%5?'#AEB8DA':'#FF642E'})}
function draw(){cx.clearRect(0,0,W,H);for(var i=0;i<Q.length;i++){var p=Q[i];p.x+=p.vx;p.y+=p.vy;if(p.x<0||p.x>W)p.vx*=-1;if(p.y<0||p.y>H)p.vy*=-1;
  for(var j=i+1;j<Q.length;j++){var q=Q[j],dx=p.x-q.x,dy=p.y-q.y,dd=dx*dx+dy*dy;if(dd<10000){cx.globalAlpha=(1-dd/10000)*.14;cx.strokeStyle='#AEB8DA';cx.beginPath();cx.moveTo(p.x,p.y);cx.lineTo(q.x,q.y);cx.stroke()}}
  var md=Math.hypot(p.x-mx,p.y-my),g=md<130?1+(1-md/130)*2:1;cx.globalAlpha=.7;cx.fillStyle=p.c;cx.beginPath();cx.arc(p.x,p.y,p.r*g,0,6.29);cx.fill()}cx.globalAlpha=1;if(!reduce)requestAnimationFrame(draw)}
var hero=$('.hero');hero.addEventListener('mousemove',function(e){var r=cv.getBoundingClientRect();mx=e.clientX-r.left;my=e.clientY-r.top});hero.addEventListener('mouseleave',function(){mx=my=-999});
addEventListener('resize',size);size();draw();
})();
