// Shared by courseware preview and the offline animation player.
const TeachingFunctions = {
  value(s, x) {
    const {a=0,b=0,c=0,k=0}=s;
    if (s.kind === 'quadratic') return a*x*x + b*x + c;
    if (s.kind === 'inverse') return x === 0 ? NaN : k/x;
    return k*x + b;
  },
  expression(s) {
    s={a:0,b:0,c:0,k:0,...s};
    const n = v => String(Number(v.toPrecision(4))).replace('-', '−');
    const term = (v, suffix) => v ? (v > 0 ? ' + ' : ' − ') + n(Math.abs(v)) + suffix : '';
    if (s.kind === 'inverse') return 'y = ' + n(s.k) + '/x';
    if (s.kind === 'quadratic') return 'y = ' + n(s.a) + 'x²' + term(s.b, 'x') + term(s.c, '') + (s.a === 0 ? '（非二次函数）' : '');
    return 'y = ' + n(s.k) + 'x' + term(s.b, '');
  },
  features(s) {
    s={a:0,b:0,c:0,k:0,...s};
    const points = [], roots = [];
    let axis;
    if (s.kind === 'inverse') return {points, roots};
    const a = s.kind === 'quadratic' ? s.a : 0, b = s.kind === 'quadratic' ? s.b : s.k, c = s.kind === 'quadratic' ? s.c : s.b;
    if (a) {
      axis = -b/(2*a);
      points.push({x:axis,y:this.value(s,axis),label:'顶点'});
      const d = b*b-4*a*c;
      if (d >= 0) { roots.push((-b-Math.sqrt(d))/(2*a)); if (d > 0) roots.push((-b+Math.sqrt(d))/(2*a)); }
    } else if (b) roots.push(-c/b);
    points.push({x:0,y:c,label:'y轴交点'});
    roots.forEach(x => { if (!points.some(p=>Math.abs(p.x-x)<1e-7 && Math.abs(p.y)<1e-7)) points.push({x,y:0,label:'x轴交点'}); });
    return {points, roots, axis};
  },
  path(s, p, X, Y, count=800) {
    let path='', previous=false, lastX;
    for(let i=0;i<=count;i++) {
      const x=p.xmin+(p.xmax-p.xmin)*i/count,y=this.value(s,x);
      if(!Number.isFinite(y)||y<p.ymin||y>p.ymax){previous=false;continue;}
      if(s.kind==='inverse' && lastX*x<=0) previous=false;
      path+=(previous?'L':'M')+X(x)+','+Y(y)+' ';previous=true;lastX=x;
    }
    return path;
  }
};
export default TeachingFunctions;
