package com.quizcraft.android;

import android.annotation.SuppressLint;
import android.app.Dialog;
import android.app.ProgressDialog;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.content.Intent;
import android.content.res.Configuration;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.graphics.Bitmap;
import android.graphics.drawable.ColorDrawable;
import android.net.ConnectivityManager;
import android.net.NetworkCapabilities;
import android.os.Build;
import android.os.Bundle;
import android.util.DisplayMetrics;
import android.view.InputDevice;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.LinearLayout;
import android.widget.TextView;
import android.widget.Toast;

import androidx.activity.OnBackPressedCallback;
import androidx.annotation.Nullable;
import androidx.appcompat.app.AppCompatActivity;
import androidx.swiperefreshlayout.widget.SwipeRefreshLayout;

import org.json.JSONObject;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.URL;

public class MainActivity extends AppCompatActivity {

    private static final String TARGET_URL = "https://superhuazai.me/practice";
    private static final String USER_AGENT_SUFFIX = " QuizCraft-Android/1.0";
    private static final int CURRENT_VERSION_CODE = 17;
    private static final String CURRENT_VERSION_NAME = "2.2.7";
    private static final String VERSION_JSON_URL = "https://gitee.com/taylorchengitee/Android-exam-solving-assistant/raw/master/version.json";
    private static final String GITHUB_URL = "https://github.com/jry21223/quizcraft-cn";
    private static final String APK_FILE_NAME = "QuizCraft-update.apk";

    private WebView webView;
    private SwipeRefreshLayout swipeRefreshLayout;
    private final Handler mainHandler = new Handler(Looper.getMainLooper());
    private static final long CHECK_INTERVAL = 20 * 60 * 1000;
    private final Runnable periodicCheck = new Runnable() {
        @Override public void run() { checkForUpdate(false); mainHandler.postDelayed(this, CHECK_INTERVAL); }
    };

    private boolean isKeyboardConnected() {
        int[] ids = InputDevice.getDeviceIds();
        for (int id : ids) {
            InputDevice dev = InputDevice.getDevice(id);
            if (dev == null || dev.isVirtual() || !dev.isExternal()) continue;
            if (dev.getKeyboardType() != InputDevice.KEYBOARD_TYPE_ALPHABETIC) continue;
            String n = dev.getName();
            if (n == null) continue;
            n = n.toLowerCase().trim();
            if (n.contains("gpio") || n.contains("headset") || n.contains("jack")
                    || n.contains("button") || n.contains("power") || n.contains("volume")
                    || n.contains("fingerprint") || n.contains("touch") || n.contains("sensor")
                    || n.contains("pen") || n.contains("stylus")) continue;
            if (dev.supportsSource(InputDevice.SOURCE_KEYBOARD)) return true;
        }
        return false;
    }

    private class KeyboardBridge {
        @JavascriptInterface
        public boolean hasKeyboard() { return isKeyboardConnected(); }
    }

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(@Nullable Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);
        initViews();
        configureWebView();
        configureSwipeRefresh();
        registerBackHandler();
        loadTargetUrl();
        checkForUpdate(false);
    }

    private void registerBackHandler() {
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override public void handleOnBackPressed() { showExitDialog(); }
        });
    }

    private void initViews() {
        swipeRefreshLayout = findViewById(R.id.swipeRefresh);
        webView = findViewById(R.id.webView);
        swipeRefreshLayout.setColorSchemeResources(R.color.primary, R.color.accent, R.color.primary_dark);
    }

    @SuppressLint("SetJavaScriptEnabled")
    private void configureWebView() {
        WebSettings s = webView.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setSupportZoom(false);
        s.setBuiltInZoomControls(false);
        s.setDisplayZoomControls(false);
        s.setCacheMode(WebSettings.LOAD_DEFAULT);
        s.setLoadWithOverviewMode(true);
        s.setUseWideViewPort(true);
        s.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        String ua = s.getUserAgentString();
        if (ua != null && !ua.contains("QuizCraft-Android")) s.setUserAgentString(ua + USER_AGENT_SUFFIX);
        webView.setWebViewClient(new QuizCraftWebViewClient());
        webView.setWebChromeClient(new QuizCraftChromeClient());
        webView.setScrollBarStyle(ViewGroup.SCROLLBARS_INSIDE_OVERLAY);
        webView.addJavascriptInterface(new KeyboardBridge(), "AndroidBridge");
    }

    private void configureSwipeRefresh() {
        swipeRefreshLayout.setOnRefreshListener(() -> {
            if (isNetworkAvailable()) webView.reload();
            else { Toast.makeText(this, "网络不可用", Toast.LENGTH_SHORT).show(); swipeRefreshLayout.setRefreshing(false); }
        });
    }

    private void loadTargetUrl() {
        if (isNetworkAvailable()) webView.loadUrl(TARGET_URL);
        else showOfflinePage();
    }

    private void showExitDialog() {
        Dialog d = new Dialog(this);
        d.requestWindowFeature(Window.FEATURE_NO_TITLE);
        d.setContentView(R.layout.dialog_exit);
        d.setCanceledOnTouchOutside(false);
        d.setCancelable(false);
        Window w = d.getWindow();
        if (w != null) {
            w.setBackgroundDrawable(new ColorDrawable(android.graphics.Color.TRANSPARENT));
            DisplayMetrics m = new DisplayMetrics();
            w.getWindowManager().getDefaultDisplay().getMetrics(m);
            w.setLayout((int)(m.widthPixels*0.85), WindowManager.LayoutParams.WRAP_CONTENT);
            w.setWindowAnimations(android.R.style.Animation_Dialog);
        }
        ((TextView)d.findViewById(R.id.version_text)).setText("当前版本：v"+CURRENT_VERSION_NAME);
        d.findViewById(R.id.view_changelog).setOnClickListener(v->{d.dismiss();viewChangelog();});
        d.findViewById(R.id.check_update).setOnClickListener(v->{d.dismiss();checkForUpdate(true);});
        final String qq="1031855485";
        View ql=d.findViewById(R.id.qq_group_layout);
        TextView qh=d.findViewById(R.id.qq_copy_hint);
        ql.setOnClickListener(v->{
            ((ClipboardManager)getSystemService(Context.CLIPBOARD_SERVICE)).setPrimaryClip(ClipData.newPlainText("QQ群",qq));
            qh.setText("已复制！快去QQ粘贴加群吧");
            qh.setTextColor(getColor(R.color.primary));
            Toast.makeText(this,"QQ群号已复制",Toast.LENGTH_SHORT).show();
        });
        d.findViewById(R.id.btn_stay).setOnClickListener(v->d.dismiss());
        d.findViewById(R.id.btn_exit).setOnClickListener(v->{d.dismiss();finishAffinity();});
        d.show();
    }

    private boolean isNetworkAvailable() {
        ConnectivityManager cm=(ConnectivityManager)getSystemService(CONNECTIVITY_SERVICE);
        if(cm==null)return false;
        if(Build.VERSION.SDK_INT>=Build.VERSION_CODES.M){
            NetworkCapabilities c=cm.getNetworkCapabilities(cm.getActiveNetwork());
            return c!=null&&(c.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)||c.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)||c.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET));
        }
        return true;
    }

    private void showOfflinePage() {
        webView.loadDataWithBaseURL(TARGET_URL,"<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"UTF-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1.0\"><title>网络不可用</title><style>*{margin:0;padding:0}body{font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;background:linear-gradient(135deg,#EFF6FF,#F9FAFB);padding:32px;text-align:center}.icon{font-size:72px;margin-bottom:24px}h2{color:#1F2937;margin-bottom:12px;font-size:22px}p{color:#6B7280;margin-bottom:24px;font-size:15px;line-height:1.6}.btn{display:inline-block;padding:12px 32px;background:#3B82F6;color:#fff;border:none;border-radius:8px;font-size:16px;cursor:pointer;text-decoration:none}.sub{margin-top:16px;font-size:12px;color:#999}</style></head><body><div class=\"icon\">&#128274;</div><h2>网络连接失败</h2><p>请检查网络连接后重试<br/>刷题数据将在联网后自动同步</p><a class=\"btn\" href=\"javascript:location.reload()\">重新加载</a><div class=\"sub\">SuperHuazai 刷题助手 &#8226; 多端协同</div></body></html>","text/html","UTF-8",null);
    }

    private void checkForUpdate(boolean showUpToDate) {
        new Thread(()->{
            try{
                URL u=new URL(VERSION_JSON_URL);
                HttpURLConnection c=(HttpURLConnection)u.openConnection();
                c.setRequestMethod("GET");c.setConnectTimeout(8000);c.setReadTimeout(8000);
                if(c.getResponseCode()!=200){if(showUpToDate)runOnUiThread(()->Toast.makeText(this,"检查更新失败",Toast.LENGTH_SHORT).show());return;}
                BufferedReader r=new BufferedReader(new InputStreamReader(c.getInputStream()));
                StringBuilder sb=new StringBuilder();String l;while((l=r.readLine())!=null)sb.append(l);r.close();c.disconnect();
                JSONObject j=new JSONObject(sb.toString());
                String latest=j.optString("latestVersionName",""),apkUrl=j.optString("apkUrl",""),msg=j.optString("updateMessage","");
                if(isNewer(latest)&&!latest.isEmpty()){
                    final String dl=apkUrl;final String m=msg;
                    runOnUiThread(()->showUpdateDialog(latest,dl,m));
                }
                else if(showUpToDate)runOnUiThread(()->Toast.makeText(this,"已是最新 v"+CURRENT_VERSION_NAME,Toast.LENGTH_SHORT).show());
            }catch(Exception e){if(showUpToDate)runOnUiThread(()->Toast.makeText(this,"检查更新失败",Toast.LENGTH_SHORT).show());}
        }).start();
    }

    private boolean isNewer(String rv){String[] ra=rv.split("\\."),ca=CURRENT_VERSION_NAME.split("\\.");int len=Math.max(ra.length,ca.length);for(int i=0;i<len;i++){int r=i<ra.length?parseIntSafe(ra[i]):0,c=i<ca.length?parseIntSafe(ca[i]):0;if(r!=c)return r>c;}return false;}
    private int parseIntSafe(String s){try{return Integer.parseInt(s);}catch(Exception e){return 0;}}
    private String formatSize(long b){if(b<1024)return b+"B";if(b<1048576)return(b/1024)+"KB";return String.format("%.1fMB",b/1048576.0);}

    // ─── MD 弹窗（统一风格：蓝顶条 + 白卡片，和退出弹窗一致）───

    private String _md(String md) {
        md = md.replace("&","&amp;").replace("<","&lt;").replace(">","&gt;");
        StringBuilder out = new StringBuilder();
        String[] lines = md.split("\n");
        boolean inUl = false;
        for (String line : lines) {
            String t = line.trim();
            if (t.startsWith("### ")) {
                if (inUl) { out.append("</ul>"); inUl = false; }
                out.append("<h3>").append(_inline(t.substring(4))).append("</h3>");
            } else if (t.startsWith("## ")) {
                if (inUl) { out.append("</ul>"); inUl = false; }
                out.append("<h2>").append(_inline(t.substring(3))).append("</h2>");
            } else if (t.startsWith("# ")) {
                if (inUl) { out.append("</ul>"); inUl = false; }
                out.append("<h1>").append(_inline(t.substring(2))).append("</h1>");
            } else if (t.startsWith("- ")) {
                if (!inUl) { out.append("<ul>"); inUl = true; }
                out.append("<li>").append(_inline(t.substring(2))).append("</li>");
            } else if (t.equals("---")) {
                if (inUl) { out.append("</ul>"); inUl = false; }
                out.append("<hr>");
            } else if (t.isEmpty()) {
                if (inUl) { out.append("</ul>"); inUl = false; }
                out.append("<br>");
            } else {
                if (inUl) { out.append("</ul>"); inUl = false; }
                out.append("<p>").append(_inline(t)).append("</p>");
            }
        }
        if (inUl) out.append("</ul>");
        return out.toString();
    }
    private String _inline(String s) {
        return s.replaceAll("\\*\\*(.+?)\\*\\*","<b>$1</b>")
                .replaceAll("`([^`]+)`","<code>$1</code>");
    }

    private int _dp(int px) { return (int)(px * getResources().getDisplayMetrics().density); }

    private void _mdDialog(String title, String md, String[] labels, Runnable[] actions) {
        Dialog d = new Dialog(this);
        d.requestWindowFeature(Window.FEATURE_NO_TITLE);
        d.setCancelable(true);
        d.setCanceledOnTouchOutside(false);
        int dw = (int)(getResources().getDisplayMetrics().widthPixels * 0.88);

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        // 圆角白色背景
        android.graphics.drawable.GradientDrawable rootBg = new android.graphics.drawable.GradientDrawable();
        rootBg.setCornerRadius(_dp(16));
        rootBg.setColor(0xFFFFFFFF);
        root.setBackground(rootBg);
        root.setClipToOutline(true);

        // 蓝色标题栏
        LinearLayout header = new LinearLayout(this);
        header.setOrientation(LinearLayout.HORIZONTAL);
        header.setBackgroundColor(0xFF3B82F6);
        header.setGravity(android.view.Gravity.CENTER_VERTICAL);
        header.setPadding(_dp(20), _dp(15), _dp(20), _dp(15));
        TextView tv = new TextView(this);
        tv.setText(title);
        tv.setTextSize(17);
        tv.setTextColor(0xFFFFFFFF);
        tv.setTypeface(null, android.graphics.Typeface.BOLD);
        header.addView(tv);
        root.addView(header, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT));

        // 正文 WebView
        WebView wv = new WebView(this);
        wv.getSettings().setJavaScriptEnabled(false);
        wv.setBackgroundColor(0x00000000);
        wv.setLayerType(View.LAYER_TYPE_NONE, null);
        String css = "body{font-family:-apple-system,'PingFang SC','Microsoft YaHei',sans-serif;"
                + "font-size:14px;color:#374151;margin:0;padding:0;line-height:1.9}"
                + "h1{font-size:16px;margin:12px 0 6px;color:#3B82F6;font-weight:700}"
                + "h2{font-size:16px;margin:16px 0 10px;color:#1F2937;font-weight:700;"
                + "padding-left:10px;border-left:3px solid #3B82F6}"
                + "h3{font-size:14px;margin:12px 0 4px;color:#1F2937;font-weight:600}"
                + "b{color:#1F2937;font-weight:600}"
                + "code{background:#EFF6FF;padding:2px 7px;border-radius:4px;"
                + "font-size:12px;color:#3B82F6;font-weight:500;white-space:nowrap}"
                + "ul{padding-left:18px;margin:6px 0}"
                + "li{margin:4px 0;color:#4B5563;line-height:1.8}"
                + "p{margin:5px 0;color:#4B5563}"
                + "hr{border:none;border-top:1px solid #E5E7EB;margin:14px 0}"
                + "br{display:block;content:'';margin-top:4px}";
        String html = "<!DOCTYPE html><html><head><meta charset=\"UTF-8\">"
                + "<meta name=\"viewport\" content=\"width=device-width,initial-scale=1.0\">"
                + "<style>" + css + "</style></head><body>" + _md(md) + "</body></html>";
        wv.loadDataWithBaseURL(null, html, "text/html", "UTF-8", null);
        LinearLayout.LayoutParams wvp = new LinearLayout.LayoutParams(
                dw - _dp(48), _dp(340));
        wvp.setMargins(_dp(24), _dp(16), _dp(24), _dp(8));
        root.addView(wv, wvp);

        // 按钮区
        if (labels != null) {
            LinearLayout btns = new LinearLayout(this);
            btns.setOrientation(LinearLayout.HORIZONTAL);
            btns.setPadding(_dp(24), _dp(4), _dp(24), _dp(18));
            for (int i = 0; i < labels.length; i++) {
                String bg = i == 0 ? "#3B82F6" : "#F3F4F6";
                String fg = i == 0 ? "#FFFFFF" : "#374151";
                TextView b = _btn(labels[i], bg, fg);
                final int idx = i;
                b.setOnClickListener(v2 -> { d.dismiss(); if (actions != null && actions[idx] != null) actions[idx].run(); });
                LinearLayout.LayoutParams bp = new LinearLayout.LayoutParams(
                        0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
                if (i > 0) bp.setMargins(_dp(8), 0, 0, 0);
                btns.addView(b, bp);
            }
            root.addView(btns);
        }

        d.setContentView(root, new ViewGroup.LayoutParams(dw, ViewGroup.LayoutParams.WRAP_CONTENT));
        Window w = d.getWindow();
        if (w != null) w.setBackgroundDrawable(new ColorDrawable(android.graphics.Color.TRANSPARENT));
        d.show();
    }

    private TextView _btn(String text, String bg, String fg) {
        TextView t = new TextView(this);
        t.setText(text); t.setTextSize(14); t.setGravity(android.view.Gravity.CENTER);
        t.setPadding(0, _dp(11), 0, _dp(11));
        t.setTextColor(android.graphics.Color.parseColor(fg));
        android.graphics.drawable.GradientDrawable gd = new android.graphics.drawable.GradientDrawable();
        gd.setCornerRadius(_dp(20));
        gd.setColor(android.graphics.Color.parseColor(bg));
        if ("#F3F4F6".equals(bg)) gd.setStroke(1, android.graphics.Color.parseColor("#E5E7EB"));
        t.setBackground(gd);
        return t;
    }

    private void viewChangelog() {
        new Thread(() -> {
            try {
                URL url = new URL(VERSION_JSON_URL);
                HttpURLConnection conn = (HttpURLConnection) url.openConnection();
                conn.setRequestMethod("GET"); conn.setConnectTimeout(8000); conn.setReadTimeout(8000);
                if (conn.getResponseCode() != 200) {
                    runOnUiThread(() -> Toast.makeText(this, "获取失败", Toast.LENGTH_SHORT).show());
                    return;
                }
                BufferedReader r = new BufferedReader(new InputStreamReader(conn.getInputStream()));
                StringBuilder sb = new StringBuilder(); String l;
                while ((l = r.readLine()) != null) sb.append(l); r.close(); conn.disconnect();
                JSONObject j = new JSONObject(sb.toString());
                String body = j.optString("updateMessage", ""), v = j.optString("latestVersionName", "");
                runOnUiThread(() -> _mdDialog("更新日志 v" + v, body, new String[]{"关闭"}, new Runnable[]{null}));
            } catch (Exception e) {
                runOnUiThread(() -> Toast.makeText(this, "获取失败", Toast.LENGTH_SHORT).show());
            }
        }).start();
    }

    private void showUpdateDialog(String v, String u, String m) {
        _mdDialog("发现新版本 v" + v, m,
            new String[]{"立即下载", "稍后再说"},
            new Runnable[]{() -> startDownload(u), null});
    }

    private void startDownload(String apkUrl) {
        ProgressDialog p = new ProgressDialog(this);
        p.setTitle("正在下载更新"); p.setMessage("准备中...");
        p.setProgressStyle(ProgressDialog.STYLE_HORIZONTAL); p.setMax(100); p.setProgress(0); p.setCancelable(false); p.show();
        new Thread(() -> {
            try {
                URL u = new URL(apkUrl); HttpURLConnection c = (HttpURLConnection) u.openConnection();
                c.setConnectTimeout(15000); c.setReadTimeout(15000); c.setInstanceFollowRedirects(true); c.connect();
                int total = c.getContentLength();
                java.io.InputStream in = c.getInputStream();
                java.io.File dir = new java.io.File(getExternalFilesDir(null), "downloads"); dir.mkdirs();
                java.io.File apk = new java.io.File(dir, APK_FILE_NAME);
                java.io.FileOutputStream out = new java.io.FileOutputStream(apk);
                byte[] buf = new byte[4096]; int r, d = 0;
                while ((r = in.read(buf)) != -1) { out.write(buf, 0, r); d += r; final int dd = d;
                    mainHandler.post(() -> { p.setMessage(formatSize(dd) + (total > 0 ? " / " + formatSize(total) : "")); if (total > 0) p.setProgress((int) ((long) dd * 100 / total)); }); }
                out.close(); in.close(); c.disconnect();
                mainHandler.post(() -> { p.dismiss(); if (apk.exists() && apk.length() > 1000) installApk(apk); else new android.app.AlertDialog.Builder(this).setTitle("下载失败").setMessage("文件不完整").setPositiveButton("确定", null).show(); });
            } catch (Exception e) { mainHandler.post(() -> { p.dismiss(); new android.app.AlertDialog.Builder(this).setTitle("下载失败").setMessage(e.getMessage()).setPositiveButton("确定", null).show(); }); }
        }).start();
    }

    private void installApk(java.io.File apk) {
        Uri uri = Build.VERSION.SDK_INT >= Build.VERSION_CODES.N ? androidx.core.content.FileProvider.getUriForFile(this, getPackageName() + ".fileprovider", apk) : Uri.fromFile(apk);
        Intent i = new Intent(Intent.ACTION_VIEW); i.setDataAndType(uri, "application/vnd.android.package-archive");
        i.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_ACTIVITY_NEW_TASK); startActivity(i);
    }

    // ═══ 键盘快捷键 JS 注入 ═══

    private void injectKeyboardShortcuts(final WebView view) {
        view.evaluateJavascript(
            "(function(){if(window.__kc4)return;window.__kc4=true;\n" +
            "var T=1500,gHeld=false,gDone=false,ni=[{k:'H',t:'首页',u:'/'},{k:'P',t:'刷题',u:'/practice'},{k:'R',t:'排行',u:'/ranking'},{k:'E',t:'工坊',u:'/extract'},{k:'W',t:'转盘',u:'/wheel'},{k:'B',t:'反馈',u:'/feedback-board'}];\n" +
            "function isFc(){var a=document.activeElement;if(!a)return 0;var t=a.tagName;return t==='INPUT'||t==='TEXTAREA'||t==='SELECT'||a.getAttribute('role')==='textbox'||a.isContentEditable;}\n" +
            "function tm(m){var e=document.getElementById('__kT');if(e)e.remove();var t=document.createElement('div');t.id='__kT';t.textContent=m;t.style.cssText='position:fixed;top:80px;left:50%;transform:translateX(-50%);background:rgba(0,0,0,0.78);color:#fff;padding:8px 18px;border-radius:8px;font-size:13px;z-index:999999;pointer-events:none;white-space:nowrap;max-width:90vw;';document.body.appendChild(t);setTimeout(function(){t.style.opacity='0';setTimeout(function(){t.remove();},300);},T);}\n" +
            "function fb(a){var b=document.querySelectorAll('button');for(var i=0;i<b.length;i++){var t=b[i].textContent.trim();for(var j=0;j<a.length;j++){if(t===a[j]||t.startsWith(a[j])){if(b[i].offsetParent!==null&&!b[i].disabled)return b[i];}}}return null;}\n" +
            "function fa(l){return document.querySelector('button[aria-label*=\"'+l+'\"]')||null;}\n" +
            "function nv(u){var a=document.querySelector('a[href=\"'+u+'\"]');if(a&&a.offsetParent!==null){a.click();return}window.location.href=u;}\n" +
            "function ck(b){if(b&&!b.disabled){b.click();return 1}return 0;}\n" +
            "function hh(){var h=document.querySelector('header');if(h&&h.offsetParent!==null)return h.offsetHeight;return 56;}\n" +
            "function fOpts(){var q=document.querySelectorAll('.overflow-hidden .flex>div');var c=null;\n" +
            "for(var i=0;i<q.length;i++){var o=q[i];if(o.offsetParent===null)continue;var d=o.firstElementChild;if(!d)continue;\n" +
            "if(d.className.indexOf('pointer-events-none')!==-1)continue;if(d.querySelector('button')){c=d;break}}\n" +
            "if(!c)c=document;var a=c.querySelectorAll('button');var r=[];\n" +
            "for(var i=0;i<a.length;i++){var b=a[i];if(b.offsetParent===null||b.disabled)continue;var t=b.textContent.trim();\n" +
            "if(t==='提交答案'||t==='上一题'||t==='下一题'||t==='查看结果')continue;\n" +
            "if(b.ariaLabel&&(b.ariaLabel.includes('收藏')||b.ariaLabel.includes('反馈')))continue;\n" +
            "if(t==='对'||t==='错'){r.push(b);continue}\n" +
            "var s=b.querySelectorAll('span');for(var j=0;j<s.length;j++){if(/^[A-D]$/.test(s[j].textContent.trim())){r.push(b);break}}}return r;}\n" +
            "function qType(){var q=document.querySelectorAll('.overflow-hidden .flex>div');for(var i=0;i<q.length;i++){if(q[i].offsetParent!==null){var t=q[i].querySelectorAll('span.text-xs.font-medium');for(var j=0;j<t.length;j++){var s=t[j].textContent.trim();if(s==='单选题'||s==='多选题'||s==='判断题'||s==='填空题')return s;}}}return'';}\n" +
            "function mBtn(){var r=[];var tags=['随机模式','难题模式','章节模式'];var b=document.querySelectorAll('button');for(var i=0;i<b.length;i++){var t=b[i].textContent.trim();for(var j=0;j<tags.length;j++){if(t.indexOf(tags[j])===0){if(b[i].offsetParent!==null)r.push(b[i]);break;}}}return r;}\n" +
            // 题库面板
            "var _bP=null,_bBtns=[],_bNames=[],_bPg=0,_bPP=7,_bF='';\n" +
            "function _bInit(){_bBtns=[];_bNames=[];var bs=document.querySelectorAll('.grid.grid-cols-2 button');for(var i=0;i<bs.length;i++){if(bs[i].offsetParent!==null&&!bs[i].disabled){_bBtns.push(bs[i]);_bNames.push(bs[i].textContent.trim().split('\\n')[0]);}}}\n" +
            "function _bRender(){\n" +
            "var fl=[];for(var i=0;i<_bNames.length;i++){if(!_bF||_bNames[i].toLowerCase().indexOf(_bF.toLowerCase())!==-1)fl.push({n:_bNames[i],idx:i});}\n" +
            "var tp=fl.length===0?1:Math.ceil(fl.length/_bPP);if(_bPg<0)_bPg=tp-1;if(_bPg>=tp)_bPg=0;\n" +
            "var st=_bPg*_bPP,pg=fl.slice(st,st+_bPP),h='';\n" +
            "h+='<div style=\"display:flex;align-items:center;justify-content:space-between;margin-bottom:8px\"><span style=\"color:rgba(255,255,255,0.4);font-size:12px\">选择题库 ('+fl.length+'个) · 按 / 搜索</span><span style=\"color:rgba(255,255,255,0.25);font-size:11px\">'+(fl.length>0?(_bPg+1)+'/'+tp:'1/1')+'</span></div>';\n" +
            "h+='<input id=\"__bS\" placeholder=\"搜索题库...\" value=\"'+_bF.replace(/\"/g,'&quot;')+'\" style=\"width:100%;padding:7px 10px;border:1px solid rgba(255,255,255,0.15);border-radius:6px;background:rgba(255,255,255,0.06);color:#e5e7eb;font-size:14px;outline:none;margin-bottom:8px;box-sizing:border-box\">';\n" +
            "if(fl.length===0){h+='<div style=\"color:rgba(255,255,255,0.25);text-align:center;padding:20px\">无匹配题库</div>';}\n" +
            "for(var i=0;i<pg.length;i++){h+='<div class=\"__bItm\" data-bidx=\"'+pg[i].idx+'\" style=\"padding:10px 12px;margin:2px 0;border-radius:6px;background:rgba(255,255,255,0.05);cursor:pointer;display:flex;align-items:center\"><span style=\"color:#93c5fd;font-weight:700;min-width:24px;font-size:14px\">'+(st+i+1)+'.</span><span style=\"color:#e5e7eb;font-size:15px\">'+pg[i].n+'</span></div>';}\n" +
            "if(fl.length>_bPP){h+='<div style=\"display:flex;align-items:center;justify-content:space-between;margin-top:10px;padding:8px 0;border-top:1px solid rgba(255,255,255,0.08)\"><span class=\"__bPrv\" style=\"color:#93c5fd;font-size:13px;cursor:pointer\">[-] 上一页</span><span style=\"color:rgba(255,255,255,0.3);font-size:12px\">'+(_bPg+1)+'/'+tp+'</span><span class=\"__bNxt\" style=\"color:#93c5fd;font-size:13px;cursor:pointer\">[+] 下一页</span></div>';}\n" +
            "if(!_bP){_bP=document.createElement('div');_bP.id='__kBk';_bP.style.cssText='position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);z-index:999998;background:#1f2937;padding:16px 18px;border-radius:14px;box-shadow:0 8px 32px rgba(0,0,0,0.45);width:340px;max-width:92vw;max-height:74vh;overflow-y:auto;font-family:system-ui';document.body.appendChild(_bP);}\n" +
            "_bP.innerHTML=h;\n" +
            "var si=document.getElementById('__bS');if(si){si.oninput=function(){_bF=this.value;_bPg=0;_bRender();};si.onkeydown=function(e){if(e.key==='Enter'){this.blur();e.stopPropagation();}else{e.stopPropagation();}};}\n" +
            "var its=_bP.querySelectorAll('.__bItm');for(var i=0;i<its.length;i++){(function(el,idx){el.onmouseover=function(){el.style.background='rgba(255,255,255,0.12)';};el.onmouseout=function(){el.style.background='rgba(255,255,255,0.05)';};el.onclick=function(){_bPick(parseInt(idx));};})(its[i],its[i].getAttribute('data-bidx'));}\n" +
            "var pv=_bP.querySelector('.__bPrv');if(pv)pv.onclick=function(){_bPg--;_bRender();};\n" +
            "var nx=_bP.querySelector('.__bNxt');if(nx)nx.onclick=function(){_bPg++;_bRender();};}\n" +
            "function _bPick(idx){if(idx>=0&&idx<_bBtns.length){ck(_bBtns[idx]);tm('题库: '+_bNames[idx]);}_bHide();}\n" +
            "function _bToggle(){if(_bP){_bHide();return;}_bInit();if(_bBtns.length===0)return;_bPg=0;_bF='';_bRender();}\n" +
            "function _bHide(){if(_bP){_bP.remove();_bP=null;}_bPg=0;_bF='';}\n" +
            // G 导航栏
            "var _gBar=null;\n" +
            "function showGBar(){if(_gBar)return;\n" +
            "var top=hh(),items=ni.map(function(x){return'<span style=\"display:inline-block;padding:6px 14px;margin:0 2px;border-radius:6px;background:rgba(255,255,255,0.08);font-size:14px;white-space:nowrap\"><b style=\"color:#fff\">'+x.k+'</b><span style=\"color:rgba(255,255,255,0.55);margin-left:4px\">'+x.t+'</span></span>';}).join('');\n" +
            "_gBar=document.createElement('div');_gBar.id='__kG';\n" +
            "_gBar.style.cssText='position:fixed;top:'+top+'px;left:0;right:0;z-index:999998;background:#1f2937;padding:10px 16px;text-align:center;line-height:1.8;box-shadow:0 2px 8px rgba(0,0,0,0.25);';\n" +
            "_gBar.innerHTML='<div style=\"max-width:600px;margin:0 auto\">'+items+'</div><div style=\"color:rgba(255,255,255,0.3);font-size:11px;margin-top:6px\">松开 G 取消 · 按住 G 同时再按上方字母跳转</div>';\n" +
            "document.body.appendChild(_gBar);}\n" +
            "function hideGBar(){if(_gBar){_gBar.remove();_gBar=null;}}\n" +
            // H 帮助面板
            "var hp=false;\n" +
            "function showHelp(){if(hp){var e=document.getElementById('__kH');if(e)e.remove();hp=false;return}\n" +
            "var d=document.createElement('div');d.id='__kH';d.innerHTML=\n" +
            "'<div style=\"position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,0.35);z-index:99998\" onclick=\"this.parentElement.remove();hp=false\"></div>'\n" +
            "+'<div style=\"position:fixed;top:50%;left:50%;transform:translate(-50%,-50%);z-index:99999;background:#fff;border-radius:16px;padding:24px 28px;max-width:360px;width:90vw;box-shadow:0 12px 48px rgba(0,0,0,0.2);font-size:13px;line-height:2\">'\n" +
            "+'<h2 style=\"margin:0 0 16px;font-size:17px;font-weight:700;color:#1e293b\">键盘快捷键</h2>'\n" +
            "+'<div style=\"color:#94a3b8;font-size:11px;font-weight:600;margin-bottom:4px\">导航</div>'\n" +
            "+'<div style=\"display:flex;flex-wrap:wrap;gap:6px;margin-bottom:14px\">'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">G + H</span><span style=\"color:#64748b;font-size:12px;line-height:2\">首页</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">G + P</span><span style=\"color:#64748b;font-size:12px;line-height:2\">刷题</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">G + R</span><span style=\"color:#64748b;font-size:12px;line-height:2\">排行</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">G + E</span><span style=\"color:#64748b;font-size:12px;line-height:2\">工坊</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">G + W</span><span style=\"color:#64748b;font-size:12px;line-height:2\">转盘</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">G + B</span><span style=\"color:#64748b;font-size:12px;line-height:2\">反馈</span>'\n" +
            "+'</div>'\n" +
            "+'<div style=\"color:#94a3b8;font-size:11px;font-weight:600;margin-bottom:4px\">刷题</div>'\n" +
            "+'<div style=\"display:flex;flex-wrap:wrap;gap:6px;margin-bottom:14px;align-items:center\">'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">1-4</span><span style=\"color:#64748b;font-size:12px\">选 ABCD</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">Space</span><span style=\"color:#64748b;font-size:12px\">提交/下一题</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">Q E</span><span style=\"color:#64748b;font-size:12px\">上下题</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">F</span><span style=\"color:#64748b;font-size:12px\">反馈</span>'\n" +
            "+'</div>'\n" +
            "+'<div style=\"color:#94a3b8;font-size:11px;font-weight:600;margin-bottom:4px\">设置</div>'\n" +
            "+'<div style=\"display:flex;flex-wrap:wrap;gap:6px;margin-bottom:14px;align-items:center\">'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">B</span><span style=\"color:#64748b;font-size:12px\">选择题库</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">1-3</span><span style=\"color:#64748b;font-size:12px\">选模式</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">Enter</span><span style=\"color:#64748b;font-size:12px\">开始</span>'\n" +
            "+'</div>'\n" +
            "+'<div style=\"color:#94a3b8;font-size:11px;font-weight:600;margin-bottom:4px\">其他</div>'\n" +
            "+'<div style=\"display:flex;flex-wrap:wrap;gap:6px;align-items:center\">'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">Esc</span><span style=\"color:#64748b;font-size:12px\">返回</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">H</span><span style=\"color:#64748b;font-size:12px\">帮助</span>'\n" +
            "+'<span style=\"background:#eff6ff;color:#3B82F6;padding:3px 10px;border-radius:6px;font-size:12px;font-weight:600\">Enter</span><span style=\"color:#64748b;font-size:12px\">再练/大转盘空格</span>'\n" +
            "+'</div>'\n" +
            "+'<p style=\"margin:14px 0 0;font-size:11px;color:#cbd5e1\">输入框聚焦时暂停 · 按住 G 显示导航</p>'\n" +
            "+'<button onclick=\"this.parentElement.parentElement.remove();hp=false\" style=\"margin-top:10px;width:100%;padding:8px;background:#3B82F6;color:#fff;border:none;border-radius:8px;font-size:13px;cursor:pointer\">关闭</button>'\n" +
            "+'</div>';document.body.appendChild(d);hp=true;}\n" +
            // 浮动按钮
            "function initFB(){if(document.getElementById('__kF'))return;\n" +
            "var hb=document.createElement('div');hb.id='__kHm';hb.title='返回首页';\n" +
            "hb.style.cssText='position:fixed;bottom:24px;right:16px;z-index:99997;width:44px;height:44px;border-radius:50%;background:#fff;color:#3B82F6;display:none;align-items:center;justify-content:center;font-size:18px;cursor:pointer;box-shadow:0 2px 12px rgba(0,0,0,0.12);border:none;';\n" +
            "hb.innerHTML='<svg width=\"20\" height=\"20\" viewBox=\"0 0 24 24\" fill=\"none\" stroke=\"currentColor\" stroke-width=\"2.2\" stroke-linecap=\"round\" stroke-linejoin=\"round\"><path d=\"M3 9.5L12 3l9 6.5V20a1 1 0 01-1 1H5a1 1 0 01-1-1V9.5z\"/><polyline points=\"9 22 9 12 15 12 15 22\"/></svg>';\n" +
            "hb.onclick=function(e){e.stopPropagation();nv('/');};document.body.appendChild(hb);\n" +
            "var kb=document.createElement('div');kb.id='__kF';kb.title='快捷键';\n" +
            "kb.style.cssText='position:fixed;bottom:76px;right:16px;z-index:99997;width:44px;height:44px;border-radius:50%;background:#3B82F6;color:#fff;display:none;align-items:center;justify-content:center;font-size:20px;cursor:pointer;box-shadow:0 2px 12px rgba(59,130,246,0.4);border:none;';\n" +
            "kb.textContent='⌨';kb.onclick=function(e){e.stopPropagation();showHelp();};document.body.appendChild(kb);}\n" +
            "function updFB(){var hm=document.getElementById('__kHm'),kb_=document.getElementById('__kF');if(!hm||!kb_)return;\n" +
            "var p=window.location.pathname;var show=(p==='/quiz'||p==='/practice'||p==='/result'||p==='/wheel');\n" +
            "hm.style.display=show?'flex':'none';\n" +
            "try{var hk=window.AndroidBridge&&window.AndroidBridge.hasKeyboard();kb_.style.display=(show&&hk)?'flex':'none';}catch(ex){kb_.style.display='none';}}\n" +
            // 键盘事件
            "document.addEventListener('keydown',function(e){\n" +
            "if(e.repeat)return;var fe=isFc();if(fe&&e.key!=='Escape')return;var k=e.key;if(e.ctrlKey||e.metaKey||e.altKey)return;var p=window.location.pathname;\n" +
            "if(k==='g'||k==='G'){if(fe)return;e.preventDefault();gHeld=true;gDone=false;showGBar();return;}\n" +
            "if(gHeld){e.preventDefault();gDone=true;hideGBar();var mi=null;for(var i=0;i<ni.length;i++){if(ni[i].k.toUpperCase()===k.toUpperCase()){mi=ni[i];break}}if(mi){nv(mi.u);tm(mi.t);}else if(k!=='Escape'){tm('无此导航: '+k);}return;}\n" +
            "if(k==='h'||k==='H'||k==='?'){e.preventDefault();showHelp();return;}\n" +
            "if(k==='Escape'){e.preventDefault();\n" +
            "if(_bP){_bHide();return;}if(_gBar){hideGBar();gHeld=false;return;}\n" +
            "var dg=document.querySelector('dialog[open]');if(dg){dg.close();return;}\n" +
            "if(p==='/quiz'||p==='/practice'||p==='/result'||p==='/wheel'){nv('/');tm('返回首页');return;}\n" +
            "return;}\n" +
            "if(p==='/quiz'){\n" +
            "if(k==='1'||k==='2'||k==='3'||k==='4'){e.preventDefault();if(qType()==='填空题'){tm('请直接键盘输入');return;}var o=fOpts(),i=parseInt(k)-1;if(i<o.length){ck(o[i]);tm(['A','B','C','D'][i]);}else tm('无'+['A','B','C','D'][i]);return;}\n" +
            "if(k===' '||k==='Space'){e.preventDefault();var s=fb(['提交答案']);if(s&&!s.disabled&&s.offsetParent!==null){ck(s);tm('已提交');return;}var n=fb(['下一题','查看结果']);if(n){ck(n);return;}tm('请先选择答案');return;}\n" +
            "if(k==='ArrowLeft'||k==='q'||k==='Q'){e.preventDefault();var pv=fb(['上一题']);if(pv&&!pv.disabled)ck(pv);else tm('已是第一题');return;}\n" +
            "if(k==='ArrowRight'||k==='e'||k==='E'){e.preventDefault();var n2=fb(['下一题','查看结果']);if(n2&&!n2.disabled)ck(n2);return;}\n" +
            "if(k==='f'||k==='F'){e.preventDefault();var fb_=fa('反馈本题');if(fb_){ck(fb_);tm('反馈');}else tm('未找到反馈按钮');return;}\n" +
            "if(k==='Enter'){e.preventDefault();var s2=fb(['提交答案']);if(s2&&!s2.disabled){ck(s2);tm('已提交');return;}var n3=fb(['下一题','查看结果']);if(n3)ck(n3);return;}\n" +
            "}\n" +
            "if(p==='/practice'){\n" +
            "if(_bP){e.preventDefault();\n" +
            "if(k==='b'||k==='B'||k==='Escape'){_bHide();return;}\n" +
            "if(k==='/'){var s=document.getElementById('__bS');if(s){s.focus();s.select();}return;}\n" +
            "var bi=parseInt(k)-1;if(k==='+'||k==='='){_bPg++;_bRender();return;}if(k==='-'){_bPg--;_bRender();return;}\n" +
            "if(!isNaN(bi)){var fl=[];for(var i=0;i<_bNames.length;i++){if(!_bF||_bNames[i].toLowerCase().indexOf(_bF.toLowerCase())!==-1)fl.push({idx:i});}var tp=fl.length===0?1:Math.ceil(fl.length/_bPP);var st=_bPg*_bPP;if(_bPg<0)_bPg=tp-1;_bRender();if(bi>=0&&bi<Math.min(_bPP,fl.length-st)){_bPick(fl[st+bi].idx);}}\n" +
            "return;}\n" +
            "if(k==='b'||k==='B'){e.preventDefault();_bToggle();return;}\n" +
            "if(k==='1'||k==='2'||k==='3'){e.preventDefault();var m=mBtn(),i=parseInt(k)-1;if(i<m.length){ck(m[i]);tm(m[i].textContent.trim().substring(0,8));}return;}\n" +
            "if(k==='Enter'){e.preventDefault();var st=fb(['开始练习']);if(st&&!st.disabled)ck(st);return;}\n" +
            "}\n" +
            "if(p==='/result'){if(k==='Enter'){e.preventDefault();var ag=fb(['再练一次']);if(ag){ck(ag);tm('再练');}else nv('/practice');}return;}\n" +
            "if(p==='/wheel'){if(k===' '||k==='Space'){e.preventDefault();var sp=fb(['抽取']);if(sp&&!sp.disabled)ck(sp);}return;}\n" +
            "});\n" +
            "document.addEventListener('keyup',function(e){if(e.key==='g'||e.key==='G'){hideGBar();gHeld=false;if(gDone)gDone=false;}});\n" +
            "function tryInit(){if(!document.body)return;initFB();updFB();\n" +
            "var obs=new MutationObserver(function(){updFB();});obs.observe(document.body,{childList:true,subtree:false});}\n" +
            "var _ti=setInterval(function(){if(document.body){clearInterval(_ti);tryInit();}},200);\n" +
            "})();", null);
    }

    private class QuizCraftWebViewClient extends WebViewClient {
        @Override public void onPageStarted(WebView v,String u,Bitmap f){super.onPageStarted(v,u,f);if(!swipeRefreshLayout.isRefreshing())swipeRefreshLayout.setRefreshing(true);}
        @Override public void onPageFinished(WebView v,String u){super.onPageFinished(v,u);swipeRefreshLayout.setRefreshing(false);injectKeyboardShortcuts(v);}
        @Override public void onReceivedError(WebView v,WebResourceRequest r,WebResourceError e){if(r!=null&&r.isForMainFrame())showOfflinePage();}
        @Override public boolean shouldOverrideUrlLoading(WebView v,WebResourceRequest r){String u=r.getUrl().toString();if(u!=null&&(u.startsWith("https://superhuazai.me")||u.startsWith("http://superhuazai.me")))return false;return false;}
    }

    private class QuizCraftChromeClient extends WebChromeClient {}

    @Override protected void onPause(){super.onPause();if(webView!=null)webView.onPause();mainHandler.removeCallbacks(periodicCheck);}
    @Override protected void onResume(){super.onResume();if(webView!=null)webView.onResume();checkForUpdate(false);mainHandler.postDelayed(periodicCheck, CHECK_INTERVAL);}
    @Override protected void onDestroy(){if(webView!=null){swipeRefreshLayout.removeView(webView);webView.removeAllViews();webView.destroy();webView=null;}super.onDestroy();}
    @Override public void onConfigurationChanged(Configuration c){super.onConfigurationChanged(c);}
}
