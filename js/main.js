
(function() {

	/* TODO
	 * [DONE] `hide selector` means hide closest selector
	 * [DONE] <div data="/xxx/xxx"> means fetch json data from /xxx/xxx
	 * [DONE] if div 'src' attr is empty, then using div.html() as template
	 * [DONE] code `r0 xxx xxx` means set reg varible. `r0/r1/r2...` can be accessed in `get/post`
	 * [DONE] <div dialog="xx xx"> means when $('#dialog') exec `ok` 'xx xx' will be executed
	 * [DONE] `show #dialog 123` <div dialog123="xx xx"> can also works
	 * [DONE] remove `onerr` stmt.
	 * [DONE] `href xx` jmp to xx
	 * <a err="xx xx"> means when error exec `xx xx`; 
	 * `post xx 'do=add'` => remove '' => `post xx do=add`
	 */

	var showerr = function (msg) {
		/*
		if (a.length) {
			a.attr('style', 'width:400px');
			a.attr('class', 'alert alert-error myerr');
			a.html(
				msg +
				'	<button type="button" class="close" data-dismiss="alert">&times;</button>'
				);
		}
	 	*/
		alert(msg);
	};

	var hideerr = function () {
		$('.myerr').hide();
	};
	
	var reload = function (d) {
		var src = d.attr('data');
		if (!src)
			return ;
		if (!src.match('\\?'))
			src += '?';
		else
			src += '&';

		var tpl = d.attr('tpl');
		
		var render = function (_tpl, _data) {
			console.log(_data);
			var html = Mustache.render(_tpl, parsejson(_data));
			d.html(html);
			binda(d);
			selall();
		};

		$.get(src, function (_data) {
			if (tpl) {
				$.get(tpl, function (_tpl) {
					render(_tpl, _data);
				});
			} else {
				render(d.html(), _data); 
			}
		});
	};

	var selall = function () {
		$('.selall').change(function () {
			var checked = $(this).attr("checked");
			if (checked) {
				$('input[type="checkbox"]').attr('checked', checked);
			} else {
				$('input[type="checkbox"]').removeAttr('checked');
			}
		});
	};

	var getdom = function (st, v) {
		var op = v.substr(0,1);
		if (op == '#')
			return $(v);
		if (op == '$') {
			var a = st.a.closest('['+v.substr(1)+']');
			var vv = a.attr(v.substr(1));
			if (vv)
				return getdom(st, vv);
		}
		if (v == 'me')
			return st.a;
		return st.a.closest(v);
	};

	var getdata = function (st, v) {
		if (v.substr(0,4) == 'form') {
			var form = st.a.closest('form');
			if (!form.length)
				form = st.a.parent().find('form');
			if (!form.length)
				return;
			if (v == 'form') 
				return form.serialize();
			if (v.substr(4,1) == '.') {
				var dom = form.find('[name="'+v.substr(5)+'"]');
				if (dom.length)
					return v.substr(5)+'='+dom.val();
				return;
			}
		}
		if (v.substr(0,1) == "'")
			return v.substr(1,v.length-2);
		if (v.substr(0,1) == '#')
			return $(v).find('form').serialize();
		if (v.match(/^r[0-9]*/))
			return $.regs[v];
	};

	var parsejson = function (str) {
		var obj;
		try {
			obj = jQuery.parseJSON(str);
		} catch (e) {
			console.log('parsejson', e);
		}
		if (!obj)
			obj = {};
		return obj;
	};

	var func_reload = function (st) {
		var d = st.p.args[1];
		if (d == 'page') {
			window.location.reload();
			return;
		}
		var dom = getdom(st, d);
		if (dom)
			reload(dom);
	};

	var func_href = function (st) {
		window.location.href = st.p.args[1];
		window.location.reload();
	};

	var func_getpost = function (st) {
		var data = '';
		for (var i = 2; i < st.p.args.length; i++) {
			var v = getdata(st, st.p.args[i]);
			if (v)
				data += v+'&';
		}
		var url = st.p.args[1];
		var dom;
		if (url.substr(0,1) != '/') {
			dom = getdom(st, url);
			if (dom)
				url = dom.attr('data');
		}
		if (!url) {
			st.cb(st);
			return;
		}
		var method = st.p.op == 'get' ? 'GET' : 'POST';
		console.log(st.p.op, url, data, dom);
		$.ajax({
			url: url,
			type: method,
			data: data,
		}).done(function (ret) {
			console.log('ajax ok');
			$.ret = ret;
			st.ret = ret;
			st.retobj = parsejson(ret);
			if (st.retobj.err)
				st.err = retobj.err;
			if (dom && method == 'GET')
				dom.html(ret);
			st.cb(st);
		}).fail(function (ret) {
			st.err = ret.responseText;
			st.cb(st);
		});
	};

	var func_hideshow = function (st) {
		var arr = st.p.args[1].split(',');
		var idx;
		if (st.p.args.length >= 3) 
			idx = st.p.args[2];
		for (var i in arr) {
			var dom = getdom(st, arr[i]);
			if (st.p.op == 'hide')
				dom.hide();
			if (st.p.op == 'show')
				dom.show();
			if (st.p.op == 'toggle')
				dom.toggle();
			if (idx)
				dom.attr('idx', idx);
		}
	};

	var func_r0 = function (st) {
		var data = '';
		for (var i = 1; i < st.p.args.length; i++) {
			var val = getdata(st, st.p.args[i]);
			if (val)
				data += val+'&';
		}
		$.regs[st.p.op] = data;
	};

	var func_ok = function (st) {
		var id = st.a.closest('div[id]').attr('id');
		if (!id)
			return;
		var idx = st.a.closest('div[id]').attr('idx');
		if (idx)
			id += idx;
		var doms = $('['+id+']');
		console.log('ok', id);
		doms.each(function () {
			var dom = $(this);
			parse(dom, id);
		});
	};

	var func_checkerr = function (st) {
		if (st.err) {
			showerr(st.err);
			return;
		}
		st.cb(st);
	};

	var func_confirm = function (st) {
		var str = st.p.args[1];
		str = str.substr(1,str.length-2);
		if (confirm(str))
			st.cb(st);
	};

	var exec = function (st) {
		if (st.i >= st.seq.length)
			return;
		st.p = st.seq[st.i++];
		console.log('exec', st.p.args, st.i+'/'+st.seq.length);
		if (st.p.op == 'ret')
			return;
		st.p.func(st);
		if (st.p.async)
			return;
		exec(st);
	};

	var parse = function (ele, attr) {
		var code = ele.attr(attr);
		if (!code) 
			return ;
		var codes = code.split(';');
		var st = {};
		st.seq = [];
		for (var i in codes) {
			var seq = {};
			seq.args = $.trim(codes[i]).split(' ');
			if (seq.args < 1)
				continue;
			console.log('parse', seq.args);

			if (seq.args[0].match(/^r[0-9]*/)) {
				if (seq.args.length < 2)
					continue;
				seq.func = func_r0;
			}
			switch (seq.args[0]) {
			case 'get':
			case 'post':
				if (seq.args.length < 2)
					continue;
				seq.func = func_getpost;
				seq.async = true;
				break;
			case 'hide':
			case 'show':
			case 'toggle':
				if (seq.args.length < 2)
					continue;
				seq.func = func_hideshow;
				break;
			case 'ret':
				break;
			case 'reload':
				if (seq.args.length < 2)
					continue;
				seq.func = func_reload;
				break;
			case 'ok':
				seq.func = func_ok;
				break;
			case 'checkerr':
				seq.func = func_checkerr;
				seq.async = true;
				break;
			case 'confirm':
				if (seq.args.length < 2)
					continue;
				seq.func = func_confirm;
				seq.async = true;
				break;
			case 'href':
				if (seq.args.length < 2)
					continue;
				seq.func = func_href;
				break;
			}
			if (seq.func) {
				seq.op = seq.args[0];
				st.seq.push(seq);
			}
		}
		console.log('click', st.seq.length);
		st.a = ele;
		st.i = 0;
		st.cb = exec;
		st.okcb_n = 0;
		exec(st);
	};

	var binda = function (div) {
		div.find('a[do]').click(function () {
			var a = $(this);
			parse(a, 'do');
		});
	};

	$(document).ready(function () {
		binda($('body'));
		selall();
		$('div[data]').each(function () {
			reload($(this));
		});
		$.regs = {};

		var q = location.hash.substring(1);
		var a = q.split(',');
		for (var i = a.length; i < 4; i++)
			a.push('');
		if (a[0] == '') {
			a[0] = 'menu';
			a[1] = 'root';
		}
		console.log(location.hash);
		switch (a[0]) {
		case 'menu':
			$('#body').attr('data', '/menu/'+a[1]);
			$('#body').attr('tpl', '/tpl/menu1.html');
			break;
		case 'vfiles':
			$('#body').attr('data', '/vfiles');
			$('#body').attr('tpl', '/tpl/vlist1.html');
			break;
		case 'vlists':
			$('#body').attr('data', '/vlists');
			$('#body').attr('tpl', '/tpl/vlist2.html');
			break;
		default:
			return;
		}
		location.hash = '#'+a.join(',');
		reload($('#body'));
	});

})();

