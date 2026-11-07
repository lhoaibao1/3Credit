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

/* hero particles */
var cv=$('#fx');if(!cv)return;var cx=cv.getContext('2d'),W=0,H=0,Q=[],mx=-999,my=-999;
function size(){var d=Math.min(2,devicePixelRatio||1),r=cv.getBoundingClientRect();W=r.width;H=r.height;cv.width=W*d;cv.height=H*d;cx.setTransform(d,0,0,d,0,0);var n=Math.round(Math.min(80,W*H/15000));Q=[];for(var i=0;i<n;i++)Q.push({x:Math.random()*W,y:Math.random()*H,vx:(Math.random()-.5)*.25,vy:(Math.random()-.5)*.25,r:.6+Math.random()*1.5,c:i%5?'#AEB8DA':'#FF642E'})}
function draw(){cx.clearRect(0,0,W,H);for(var i=0;i<Q.length;i++){var p=Q[i];p.x+=p.vx;p.y+=p.vy;if(p.x<0||p.x>W)p.vx*=-1;if(p.y<0||p.y>H)p.vy*=-1;
  for(var j=i+1;j<Q.length;j++){var q=Q[j],dx=p.x-q.x,dy=p.y-q.y,dd=dx*dx+dy*dy;if(dd<10000){cx.globalAlpha=(1-dd/10000)*.14;cx.strokeStyle='#AEB8DA';cx.beginPath();cx.moveTo(p.x,p.y);cx.lineTo(q.x,q.y);cx.stroke()}}
  var md=Math.hypot(p.x-mx,p.y-my),g=md<130?1+(1-md/130)*2:1;cx.globalAlpha=.7;cx.fillStyle=p.c;cx.beginPath();cx.arc(p.x,p.y,p.r*g,0,6.29);cx.fill()}cx.globalAlpha=1;if(!reduce)requestAnimationFrame(draw)}
var hero=$('.hero');hero.addEventListener('mousemove',function(e){var r=cv.getBoundingClientRect();mx=e.clientX-r.left;my=e.clientY-r.top});hero.addEventListener('mouseleave',function(){mx=my=-999});
addEventListener('resize',size);size();draw();
})();
