package com.reasonix.mobile.installfix;

import android.Manifest;
import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.os.Bundle;
import android.speech.RecognitionListener;
import android.speech.RecognizerIntent;
import android.speech.SpeechRecognizer;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import java.util.ArrayList;
import java.util.Locale;
import org.json.JSONObject;

public final class MainActivity extends Activity {
    private static final int REQ_RECORD_AUDIO = 7001;
    private WebView webView;
    private SpeechRecognizer recognizer;
    private String pendingLocale = "ru-RU";
    private boolean voiceActive = false;
    private final Runnable voiceTimeout = new Runnable() {
        @Override public void run() {
            if (!voiceActive) return;
            if (recognizer != null) {
                try { recognizer.cancel(); } catch (RuntimeException ignored) { }
            }
            voiceActive = false;
            js("onError", "recognition_timeout");
            destroyRecognizer();
        }
    };

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setStatusBarColor(Color.BLACK);
        getWindow().setNavigationBarColor(Color.BLACK);

        webView = new WebView(this);
        webView.setBackgroundColor(Color.rgb(11, 11, 12));
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setDatabaseEnabled(true);
        settings.setAllowFileAccess(true);
        settings.setAllowContentAccess(true);
        settings.setAllowFileAccessFromFileURLs(true);
        settings.setAllowUniversalAccessFromFileURLs(true);

        webView.setWebViewClient(new WebViewClient());
        webView.setWebChromeClient(new WebChromeClient());
        webView.addJavascriptInterface(new NativeBridge(this), "ReasonixNative");
        setContentView(webView);
        webView.loadUrl("file:///android_asset/index.html");
    }

    public final class NativeBridge {
        private final Context context;
        NativeBridge(Context context) { this.context = context; }

        @JavascriptInterface
        public void startVoice(final String localeTag) {
            runOnUiThread(new Runnable() {
                @Override public void run() { beginVoice(localeTag); }
            });
        }

        @JavascriptInterface
        public void cancelVoice() {
            runOnUiThread(new Runnable() {
                @Override public void run() { cancelVoiceInternal(); }
            });
        }

        @JavascriptInterface
        public boolean voiceAvailable() {
            return SpeechRecognizer.isRecognitionAvailable(context);
        }
    }

    private void beginVoice(String localeTag) {
        if (voiceActive) {
            cancelVoiceInternal();
            return;
        }
        pendingLocale = normalizeLocale(localeTag);
        if (!SpeechRecognizer.isRecognitionAvailable(this)) {
            js("onError", "recognizer_unavailable");
            return;
        }
        if (android.os.Build.VERSION.SDK_INT >= 23 && checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.RECORD_AUDIO}, REQ_RECORD_AUDIO);
            return;
        }
        startRecognizer();
    }

    private String normalizeLocale(String localeTag) {
        if (localeTag == null || localeTag.trim().isEmpty()) return Locale.getDefault().toLanguageTag();
        String tag = localeTag.trim();
        if (tag.length() > 32) return Locale.getDefault().toLanguageTag();
        return tag;
    }

    private void startRecognizer() {
        destroyRecognizer();
        recognizer = SpeechRecognizer.createSpeechRecognizer(this);
        recognizer.setRecognitionListener(new RecognitionListener() {
            @Override public void onReadyForSpeech(Bundle params) { }
            @Override public void onBeginningOfSpeech() { }
            @Override public void onRmsChanged(float rmsdB) { }
            @Override public void onBufferReceived(byte[] buffer) { }
            @Override public void onEndOfSpeech() { }
            @Override public void onError(int error) {
                finishVoiceTimer();
                voiceActive = false;
                js("onError", errorName(error));
                destroyRecognizer();
            }
            @Override public void onResults(Bundle results) {
                finishVoiceTimer();
                voiceActive = false;
                String text = firstResult(results);
                if (text.isEmpty()) js("onError", "no_match");
                else js("onFinal", text);
                destroyRecognizer();
            }
            @Override public void onPartialResults(Bundle partialResults) {
                String text = firstResult(partialResults);
                if (!text.isEmpty()) js("onPartial", text);
            }
            @Override public void onEvent(int eventType, Bundle params) { }
        });
        Intent intent = new Intent(RecognizerIntent.ACTION_RECOGNIZE_SPEECH);
        intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE_MODEL, RecognizerIntent.LANGUAGE_MODEL_FREE_FORM);
        intent.putExtra(RecognizerIntent.EXTRA_PARTIAL_RESULTS, true);
        intent.putExtra(RecognizerIntent.EXTRA_LANGUAGE, pendingLocale);
        intent.putExtra(RecognizerIntent.EXTRA_MAX_RESULTS, 3);
        try {
            recognizer.startListening(intent);
            voiceActive = true;
            js("onStart", null);
            finishVoiceTimer();
            if (webView != null) webView.postDelayed(voiceTimeout, 35000L);
        } catch (RuntimeException e) {
            voiceActive = false;
            destroyRecognizer();
            js("onError", "start_failed");
        }
    }

    private void finishVoiceTimer() {
        if (webView != null) webView.removeCallbacks(voiceTimeout);
    }

    private String firstResult(Bundle bundle) {
        if (bundle == null) return "";
        ArrayList<String> values = bundle.getStringArrayList(SpeechRecognizer.RESULTS_RECOGNITION);
        if (values == null || values.isEmpty() || values.get(0) == null) return "";
        return values.get(0).trim();
    }

    private String errorName(int code) {
        switch (code) {
            case SpeechRecognizer.ERROR_AUDIO: return "audio";
            case SpeechRecognizer.ERROR_CLIENT: return "client";
            case SpeechRecognizer.ERROR_INSUFFICIENT_PERMISSIONS: return "permission_denied";
            case SpeechRecognizer.ERROR_NETWORK: return "network";
            case SpeechRecognizer.ERROR_NETWORK_TIMEOUT: return "network_timeout";
            case SpeechRecognizer.ERROR_NO_MATCH: return "no_match";
            case SpeechRecognizer.ERROR_RECOGNIZER_BUSY: return "recognizer_busy";
            case SpeechRecognizer.ERROR_SERVER: return "server";
            case SpeechRecognizer.ERROR_SPEECH_TIMEOUT: return "speech_timeout";
            default: return "error_" + code;
        }
    }

    private void cancelVoiceInternal() {
        finishVoiceTimer();
        if (recognizer != null) {
            try { recognizer.cancel(); } catch (RuntimeException ignored) { }
        }
        voiceActive = false;
        js("onCancel", null);
        destroyRecognizer();
    }

    private void destroyRecognizer() {
        if (recognizer != null) {
            try { recognizer.destroy(); } catch (RuntimeException ignored) { }
            recognizer = null;
        }
    }

    private void js(final String method, final String value) {
        if (webView == null) return;
        final String script;
        if (value == null) script = "window.ReasonixVoice&&window.ReasonixVoice." + method + "&&window.ReasonixVoice." + method + "();";
        else script = "window.ReasonixVoice&&window.ReasonixVoice." + method + "&&window.ReasonixVoice." + method + "(" + JSONObject.quote(value) + ");";
        webView.post(new Runnable() {
            @Override public void run() { if (webView != null) webView.evaluateJavascript(script, null); }
        });
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode != REQ_RECORD_AUDIO) return;
        if (grantResults.length > 0 && grantResults[0] == PackageManager.PERMISSION_GRANTED) startRecognizer();
        else js("onError", "permission_denied");
    }

    @Override
    protected void onDestroy() {
        finishVoiceTimer();
        voiceActive = false;
        destroyRecognizer();
        if (webView != null) {
            webView.removeJavascriptInterface("ReasonixNative");
            webView.destroy();
            webView = null;
        }
        super.onDestroy();
    }
}
