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

/* nav shadow after scrolling */
var nv=$('.nav');if(nv){var sc=function(){nv.classList.toggle('scrolled',scrollY>8)};sc();addEventListener('scroll',sc,{passive:true})}

/* banner slider: arrows, dots, auto-advance that pauses on hover/focus and is off for reduced motion */
var hs=$('#hbSlides');
if(hs){var sl=$$('.hb-copy',hs),dots=$$('#hbDots button'),hi=0,tm=null,
  go=function(n){hi=(n+sl.length)%sl.length;sl.forEach(function(x,k){x.classList.toggle('on',k===hi)});dots.forEach(function(d,k){d.classList.toggle('on',k===hi)})},
  play=function(){if(reduce||tm)return;tm=setInterval(function(){go(hi+1)},6500)},stop=function(){clearInterval(tm);tm=null};
  $$('[data-hb]').forEach(function(b){b.addEventListener('click',function(){stop();go(hi+ +b.dataset.hb);play()})});
  dots.forEach(function(d,k){d.addEventListener('click',function(){stop();go(k);play()})});
  var hbx=hs.closest('.hb');hbx.addEventListener('mouseenter',stop);hbx.addEventListener('mouseleave',play);hbx.addEventListener('focusin',stop);hbx.addEventListener('focusout',play);play()}

/* mobile menu: Esc, outside click and link click close it */
var mn=$('details.mnav');
if(mn){var shut=function(f){if(mn.open){mn.removeAttribute('open');if(f)$('summary',mn).focus()}};
  document.addEventListener('keydown',function(e){if(e.key==='Escape')shut(true)});
  document.addEventListener('click',function(e){if(!mn.contains(e.target))shut();else if(e.target.closest('.mnav-panel a'))shut()})}

/* CMS tabs (mobile row): bring the current tab into view without scrolling the page */
var an=$('nav.anav'),tabCur=an&&$('[aria-current]',an);
if(tabCur&&innerWidth<1024)an.scrollLeft=tabCur.offsetLeft-an.offsetLeft-(an.clientWidth-tabCur.offsetWidth)/2;

/* live matching */
var form=$('#pf');
/* question flow: one step at a time, results only after the final submit (server renders them with xem=1) */
if(form){
  var steps=$$('.wz-step',form),cur=0,err=$('#wzErr');
  var money=function(m){m=+m;return m>=1000?(m/1000).toLocaleString('vi-VN')+' tỷ':m+' triệu'};
  var labels=function(){$('#oAmount').textContent=money($('#fAmount').value);$('#oTerm').textContent=$('#fTerm').value+' tháng'};
  var need=function(){var c=$('input[name=need]:checked',form);return c?c.value:''};
  var mode=function(){var n=need();form.classList.toggle('card',n==='credit_card');form.classList.toggle('veh',n==='secured'||n==='compare')};
  var show=function(i,focus){var to=Math.max(0,Math.min(i,steps.length-1));form.classList.toggle('back',to<cur);cur=to;steps.forEach(function(s,k){s.hidden=k!==cur});err.hidden=true;
    $('#wzN').textContent=cur+1;$('#wzBar').style.width=((cur+1)/steps.length*100)+'%';
    form.classList.toggle('first',cur===0);form.classList.toggle('last',cur===steps.length-1);
    if(focus){var f=$('input:checked,input:not([type=hidden]):not([type=radio]),.cs-btn,input[type=radio]',steps[cur]);if(f)f.focus({preventScroll:true});form.scrollIntoView({block:'start'})}};
  var ok=function(){var bad=$$('[data-req]',steps[cur]).filter(function(g){return !$('input:checked',g)});
    if(cur===0&&!need())bad.push(steps[0]);err.hidden=!bad.length;if(bad.length){var i=$('input',bad[0]);if(i)i.focus()}return !bad.length};
  var next=function(){if(!ok())return;if(cur===steps.length-1)form.requestSubmit?form.requestSubmit():form.submit();else show(cur+1,true)};
  form.classList.add('js');mode();labels();show(+form.dataset.start-1||0,false);
  form.addEventListener('input',labels);
  /* picking the need with mouse or touch moves on by itself (keyboard users press Enter) */
  steps[0].addEventListener('click',function(e){var l=e.target.closest('.opt');if(l&&e.target.tagName==='INPUT'&&e.detail>0&&cur===0)setTimeout(function(){if(cur===0)next()},260)});
  form.addEventListener('change',function(e){if(e.target.name==='need')mode();if(e.target.type==='radio')err.hidden=true});
  $('[data-next]',form).addEventListener('click',next);
  $('[data-back]',form).addEventListener('click',function(){show(cur-1,true)});
  form.addEventListener('submit',function(e){if(!ok()){e.preventDefault();return}var d=$('[data-done]',form);if(d){d.setAttribute('aria-busy','true');d.textContent='Đang đối chiếu…'}});
  addEventListener('pageshow',function(){var d=$('[data-done]',form);if(d&&d.getAttribute('aria-busy')){d.removeAttribute('aria-busy');d.textContent='Xem gói vay phù hợp'}});
  form.addEventListener('keydown',function(e){if(e.key==='Enter'&&!e.isComposing&&e.target.tagName!=='BUTTON'&&!e.target.closest('.cs')){e.preventDefault();next()}});
  var wzOpen=function(i){var p=$('#psum');if(p)p.hidden=true;var r=$('#results');if(r)r.hidden=true;form.hidden=false;show(i,true)};
  document.addEventListener('click',function(e){var a=e.target.closest&&e.target.closest('[data-edit],[data-pick-need]');if(!a)return;e.preventDefault();
    if(a.dataset.pickNeed){var r=$('input[name=need][value="'+a.dataset.pickNeed+'"]',form);if(r){r.checked=true;mode()}
      var v=a.dataset.pickVeh&&$('input[name=vehicle][value="'+a.dataset.pickVeh+'"]',form);if(v)v.checked=true;
      wzOpen(1)}else wzOpen(0)});
}

/* compare tray */
var cmp=[];
function tray(){var t=$('#tray');if(!t)return;t.hidden=cmp.length===0;$('#trayTxt').textContent='Đã chọn '+cmp.length+'/3';$('#trayGo').disabled=cmp.length<2}
function syncCmp(){$$('[data-cmp]').forEach(function(c){c.checked=cmp.indexOf(c.dataset.cmp)>=0});tray()}
document.addEventListener('change',function(e){var c=e.target.closest&&e.target.closest('[data-cmp]');if(!c)return;var id=c.dataset.cmp,i=cmp.indexOf(id);
  if(c.checked&&i<0){if(cmp.length>=3){c.checked=false;return}cmp.push(id)}if(!c.checked&&i>=0)cmp.splice(i,1);tray()});
var tg2=$('#trayGo');if(tg2)tg2.addEventListener('click',function(){var q=form?new URLSearchParams(new FormData(form)):new URLSearchParams();q.set('ids',cmp.join(','));location.href='/so-sanh?'+q.toString()});
var tc=$('#trayClr');if(tc)tc.addEventListener('click',function(){cmp=[];syncCmp()});

/* scroll effects: sections rise into view (staggered), a progress line, hero parallax, count-up, back-to-top */
if(!reduce&&'IntersectionObserver' in window){
  var io=new IntersectionObserver(function(en){en.forEach(function(x){if(x.isIntersecting){x.target.classList.remove('pre');io.unobserve(x.target)}})},{threshold:.12,rootMargin:'0px 0px -6% 0px'});
  var arm=function(el,kind,delay){if(el.classList.contains('rv'))return;el.classList.add('rv');if(kind)el.classList.add(kind);
    if(el.getBoundingClientRect().top>innerHeight*.92){if(delay)el.style.transitionDelay=delay+'ms';el.classList.add('pre');io.observe(el)}};
  $$('.reveal').forEach(function(el){el.classList.remove('reveal');});
  [['.sh',''],['.split-t','fromL'],['.about-t','fromR'],['.about-art','zoom'],['.wz',''],['.wz-art','fromL'],['#psum',''],['.consult',''],['.ctr-btn',''],['.cta-card',''],['.alert',''],['footer .in > div','']].forEach(function(r){$$(r[0]).forEach(function(el){arm(el,r[1],0)})});
  [['.pgrid','.pcard',140],['.pc-list','li',90],['.flow','.fs',110],['.posts','.post',110],['.faq','details',80],['.about-f','li',100],['.hb-tiles','li',120],['.warn-grid, .wgrid','> *',90]].forEach(function(g){
    $$(g[0]).forEach(function(box){$$(g[1].charAt(0)==='>'?':scope '+g[1]:g[1],box).forEach(function(el,i){arm(el,'',Math.min(i,7)*g[2])})})});
}
var bar=document.createElement('div');bar.id='topbar';document.body.appendChild(bar);
var tt=document.createElement('button');tt.className='totop';tt.type='button';tt.setAttribute('aria-label','Lên đầu trang');
tt.innerHTML='<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 14l7-7 7 7"/></svg>';
document.body.appendChild(tt);tt.addEventListener('click',function(){scrollTo({top:0,behavior:reduce?'auto':'smooth'})});
var stack=$('.hb .cardstack'),tick=false;
function onScroll(){tick=false;var y=scrollY,max=document.documentElement.scrollHeight-innerHeight;
  bar.style.transform='scaleX('+(max>0?Math.min(1,y/max):0)+')';tt.classList.toggle('on',y>700);
  if(stack&&!reduce&&y<900)stack.style.transform='translateY('+(y*.18)+'px) rotate('+(y*.01)+'deg)'}
addEventListener('scroll',function(){if(!tick){tick=true;requestAnimationFrame(onScroll)}},{passive:true});onScroll();
/* numbers in the banner tiles count up once */
if(!reduce)$$('.hb-tiles b').forEach(function(el){var n=parseInt(el.textContent,10);if(!(n>1)||String(n)!==el.textContent.trim())return;var t0=null;
  var step=function(t){if(!t0)t0=t;var k=Math.min(1,(t-t0)/900);el.textContent=Math.round(n*(1-Math.pow(1-k,3)));if(k<1)requestAnimationFrame(step)};el.textContent='0';setTimeout(function(){requestAnimationFrame(step)},500)});

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
