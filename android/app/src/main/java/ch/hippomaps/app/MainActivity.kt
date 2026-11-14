package ch.hippomaps.app

import android.Manifest
import android.annotation.SuppressLint
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Color
import android.net.ConnectivityManager
import android.net.Network
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.View
import android.view.WindowManager
import android.webkit.CookieManager
import android.webkit.GeolocationPermissions
import android.webkit.PermissionRequest
import android.webkit.RenderProcessGoneDetail
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.Button
import android.widget.FrameLayout
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.OnBackPressedCallback
import androidx.activity.SystemBarStyle
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.lifecycle.Lifecycle
import androidx.webkit.WebViewCompat
import androidx.webkit.WebViewFeature
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import kotlin.concurrent.thread

class MainActivity : ComponentActivity() {

    companion object {
        const val HOST = "hippomaps.ch"
        const val ORIGIN = "https://$HOST"
        const val HOME = "$ORIGIN/"
        private const val LIGHT_BG = 0xFFFFFFFF.toInt() // --bg of the page
        private const val DARK_BG = 0xFF17191C.toInt()
        private const val SPLASH_MAX_MS = 10_000L
        private const val PREFS = "app"
        private const val PREF_DARK = "dark"
        private const val PREF_ASKED_NOTIFICATIONS = "asked_notifications"
        private const val GEO_DENIED = """{"type":"geoerr","code":1,"message":"User denied Geolocation"}"""
        private const val GEO_UNAVAILABLE = """{"type":"geoerr","code":2,"message":"Location is turned off"}"""

        // no user info and no backslash: Uri reads "https://evil.example\@hippomaps.ch/" as hippomaps.ch,
        // the WebView as evil.example
        fun isOurs(uri: Uri?): Boolean =
            uri != null && uri.scheme == "https" && uri.host == HOST && (uri.port == -1 || uri.port == 443) &&
                uri.userInfo == null && '\\' !in uri.toString()

        // schemes a link of the page may not hand to other apps
        private val BLOCKED_SCHEMES = setOf("file", "content", "javascript", "data", "blob", "about")
    }

    private lateinit var root: FrameLayout
    private lateinit var web: WebView
    private lateinit var errorView: View
    private val handler = Handler(Looper.getMainLooper())

    private var ready = false // the splash screen goes once the first page is shown (or the error screen)
    private var pageFailed = false
    private var failedUrl: String? = null
    private var clearHistoryWhenLoaded = false
    private var dark = false
    private val wakeLocks = mutableSetOf<Int>()
    private var webGone = false // the renderer crashed or was killed: the WebView is destroyed

    private var pendingGeo: Pair<String, GeolocationPermissions.Callback>? = null
    // Android shows one permission dialog at a time; a second request meanwhile comes back denied
    private var askingLocation = false
    private var askingNotifications = false
    private var locationAfterNotifications = false
    private val askLocation = registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { result ->
        askingLocation = false
        val granted = result.values.any { it } || hasLocation()
        pendingGeo?.let { (origin, callback) -> callback.invoke(origin, granted, granted) }
        pendingGeo = null
        if (geoWanted) {
            if (granted) startFeed() else toPage?.invoke(GEO_DENIED)
        }
        if (granted && navigating) {
            startNavigationService()
            askNotificationsOnce()
        }
    }
    private val askNotifications = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        askingNotifications = false
        if (granted) NavigationService.instance?.repost() // posted before the answer: show it now
        if (locationAfterNotifications) {
            locationAfterNotifications = false
            requestLocation()
        }
    }
    private val endNavFallback = Runnable { if (navigating) setNavigating(false) }

    // the page: messages to it go through the reply channel of its last message
    private var toPage: ((String) -> Unit)? = null
    private lateinit var feed: LocationFeed
    private var geoWanted = false // the page has a position request or a watch open
    private lateinit var compass: Compass
    private var compassWanted = false // the page shows the heading of the phone
    private var navigating = false

    private val network = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(n: Network) {
            handler.post { if (pageFailed) retry() }
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        val splash = installSplashScreen()
        super.onCreate(savedInstanceState)
        splash.setKeepOnScreenCondition { !ready }
        handler.postDelayed({ ready = true }, SPLASH_MAX_MS)

        dark = getSharedPreferences(PREFS, MODE_PRIVATE).getBoolean(PREF_DARK, isSystemDark())
        setContentView(R.layout.activity_main)
        root = findViewById(R.id.root)
        web = findViewById(R.id.web)
        errorView = findViewById(R.id.error)
        findViewById<Button>(R.id.retry).setOnClickListener { retry() }
        applyTheme(dark)

        // edge to edge: the page sits between the system bars, the bars show the page background
        ViewCompat.setOnApplyWindowInsetsListener(root) { v, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
            val ime = insets.getInsets(WindowInsetsCompat.Type.ime())
            v.setPadding(bars.left, bars.top, bars.right, maxOf(bars.bottom, ime.bottom))
            WindowInsetsCompat.CONSUMED
        }

        compass = Compass(this, ::screenRotation) { msg -> toPage?.invoke(msg) }
        feed = LocationFeed(this, { compass.at(it) }) { msg -> toPage?.invoke(msg) }
        NavigationService.onStopRequested = { endNavigationInPage() }
        setUpWebView()

        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() = back(this)
        })

        getSystemService(ConnectivityManager::class.java)?.registerDefaultNetworkCallback(network)

        val start = intent?.takeIf { it.action == Intent.ACTION_VIEW }?.data?.toString()?.let(::linkOf)
        if (savedInstanceState != null && start == null && web.restoreState(savedInstanceState) != null) {
            // restored
        } else {
            web.loadUrl(start ?: HOME)
            if (start == null && savedInstanceState == null) intent?.let { openLater(it) }
        }
    }

    @SuppressLint("SetJavaScriptEnabled")
    private fun setUpWebView() {
        WebView.setWebContentsDebuggingEnabled(BuildConfig.DEBUG)
        web.setBackgroundColor(if (dark) DARK_BG else LIGHT_BG)
        web.overScrollMode = View.OVER_SCROLL_NEVER
        web.isVerticalScrollBarEnabled = false
        web.isHorizontalScrollBarEnabled = false
        with(web.settings) {
            javaScriptEnabled = true
            domStorageEnabled = true
            databaseEnabled = true
            mediaPlaybackRequiresUserGesture = false
            setGeolocationEnabled(true)
            setSupportZoom(false)
            builtInZoomControls = false
            displayZoomControls = false
            allowFileAccess = false
            allowContentAccess = false
            mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
            userAgentString = "$userAgentString hippomaps-android/${BuildConfig.VERSION_NAME}"
        }
        CookieManager.getInstance().setAcceptCookie(true)

        val origins = setOf(ORIGIN)
        if (WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)) {
            WebViewCompat.addWebMessageListener(web, "hmNative", origins) { _, message, sourceOrigin, isMainFrame, reply ->
                if (isMainFrame && sourceOrigin.toString().trimEnd('/') == ORIGIN) {
                    onPageMessage(message.data ?: return@addWebMessageListener) { answer -> reply.postMessage(answer) }
                }
            }
        }
        if (WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)) {
            val script = assets.open("bridge.js").bufferedReader().use { it.readText() }
            WebViewCompat.addDocumentStartJavaScript(web, script, origins)
        }

        web.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                if (!request.isForMainFrame) return false
                if (isOurs(request.url)) return false
                openOutside(request.url)
                return true
            }

            override fun onPageStarted(view: WebView, url: String?, favicon: Bitmap?) {
                pageFailed = false
                releaseWakeLocks()
                // a new document: no position requests, no navigation until it says so
                toPage = null
                geoWanted = false
                feed.stop()
                compassWanted = false
                compass.stop()
                setNavigating(false)
            }

            override fun onPageCommitVisible(view: WebView, url: String?) {
                if (!pageFailed) showPage()
            }

            override fun onPageFinished(view: WebView, url: String?) {
                if (!pageFailed) showPage()
                if (clearHistoryWhenLoaded) {
                    clearHistoryWhenLoaded = false
                    view.clearHistory()
                }
                CookieManager.getInstance().flush()
            }

            override fun onReceivedError(view: WebView, request: WebResourceRequest, error: WebResourceError) {
                if (request.isForMainFrame) showError(request.url.toString())
            }

            // the renderer crashed or the system took its memory: without this the app would be killed
            // with it. A new activity loads the page again (an ongoing navigation ends).
            override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
                if (webGone) return true
                webGone = true
                // the new activity opens the page that was shown
                view.url?.let { Uri.parse(it) }?.takeIf { isOurs(it) }?.let { intent = Intent(intent).setData(it) }
                root.removeView(view)
                view.destroy()
                recreate()
                return true
            }

            override fun onReceivedHttpError(view: WebView, request: WebResourceRequest, response: WebResourceResponse) {
                if (request.isForMainFrame && response.statusCode >= 500) showError(request.url.toString())
            }
        }

        web.webChromeClient = object : WebChromeClient() {
            override fun onGeolocationPermissionsShowPrompt(origin: String, callback: GeolocationPermissions.Callback) {
                if (!isOurs(Uri.parse(origin))) {
                    callback.invoke(origin, false, false)
                    return
                }
                if (hasLocation()) {
                    callback.invoke(origin, true, true)
                    return
                }
                pendingGeo = origin to callback
                requestLocation()
            }

            override fun onGeolocationPermissionsHidePrompt() {
                pendingGeo = null
            }

            override fun onPermissionRequest(request: PermissionRequest) {
                request.deny() // no camera or microphone
            }
        }

        web.setDownloadListener { url, _, _, _, _ -> openOutside(Uri.parse(url)) }
    }

    // ---------------------------------------------------------------- messages from the page
    private fun onPageMessage(data: String, reply: (String) -> Unit) {
        val m = try { JSONObject(data) } catch (e: Exception) { return }
        toPage = reply
        when (m.optString("type")) {
            "hello" -> Unit
            "geo" -> {
                geoWanted = m.optBoolean("watch")
                if (!geoWanted) feed.stop()
                else if (hasLocation()) startFeed()
                else requestLocation()
            }
            "compass" -> {
                compassWanted = m.optBoolean("on")
                if (compassWanted && lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) compass.start() else compass.stop()
            }
            "nav" -> setNavigating(m.optBoolean("on"))
            "navinfo" -> NavigationService.instance?.update(m.optString("dist"), m.optString("instr"))
            "share" -> {
                val id = m.optInt("id")
                val title = m.optString("title")
                val text = listOf(m.optString("text"), m.optString("url")).filter { it.isNotBlank() }.joinToString("\n")
                val send = Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    putExtra(Intent.EXTRA_TEXT, text.ifBlank { title })
                    if (title.isNotBlank()) {
                        putExtra(Intent.EXTRA_SUBJECT, title)
                        putExtra(Intent.EXTRA_TITLE, title)
                    }
                }
                val answer = JSONObject().put("id", id)
                try {
                    startActivity(Intent.createChooser(send, title.ifBlank { null }))
                    answer.put("ok", true)
                } catch (e: Exception) { // no app, or the text too large for an intent
                    answer.put("ok", false).put("error", "NotAllowedError")
                }
                reply(answer.toString())
            }
            "wake" -> {
                val id = m.optInt("id")
                if (m.optBoolean("on")) wakeLocks.add(id) else wakeLocks.remove(id)
                keepScreenOn(wakeLocks.isNotEmpty())
            }
            "theme" -> {
                val d = m.optBoolean("dark")
                if (d != dark) {
                    dark = d
                    applyTheme(d)
                    getSharedPreferences(PREFS, MODE_PRIVATE).edit().putBoolean(PREF_DARK, d).apply()
                }
            }
        }
    }

    // ---------------------------------------------------------------- location and navigation
    private fun requestLocation() {
        if (askingLocation) return
        if (askingNotifications) {
            locationAfterNotifications = true
            return
        }
        askingLocation = true
        askLocation.launch(arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION))
    }

    private fun startFeed() {
        val fine = ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        if (!feed.start(fine)) toPage?.invoke(GEO_UNAVAILABLE)
    }

    private fun setNavigating(on: Boolean) {
        if (on == navigating) return
        navigating = on
        handler.removeCallbacks(endNavFallback)
        if (on) {
            // without location the page asks for it first; the notifications follow its answer
            if (hasLocation()) {
                startNavigationService()
                askNotificationsOnce()
            }
            // the page renderer keeps its priority while the app is in the background
            web.setRendererPriorityPolicy(WebView.RENDERER_PRIORITY_IMPORTANT, false)
        } else {
            NavigationService.stop(this)
            web.setRendererPriorityPolicy(WebView.RENDERER_PRIORITY_IMPORTANT, true)
            // ended with the app in the background (Stop, arrival): no positions and timers there
            if (!lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) feed.stop()
            if (!lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED) && !webGone) web.onPause()
        }
    }

    private fun startNavigationService() {
        try {
            NavigationService.start(this)
        } catch (e: Exception) {
            // not allowed from the background (Android 12+): navigation runs while the app is in front
        }
    }

    private fun askNotificationsOnce() {
        if (Build.VERSION.SDK_INT < 33) return
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED) return
        val prefs = getSharedPreferences(PREFS, MODE_PRIVATE)
        if (prefs.getBoolean(PREF_ASKED_NOTIFICATIONS, false) || askingLocation || askingNotifications) return
        prefs.edit().putBoolean(PREF_ASKED_NOTIFICATIONS, true).apply()
        askingNotifications = true
        askNotifications.launch(Manifest.permission.POST_NOTIFICATIONS)
    }

    private fun endNavigationInPage() {
        web.evaluateJavascript("window.__hmEndNav && window.__hmEndNav()", null)
        // the page answers with {type: "nav", on: false}; the service goes in any case
        handler.removeCallbacks(endNavFallback)
        handler.postDelayed(endNavFallback, 2000)
    }

    private fun applyTheme(dark: Boolean) {
        val bg = if (dark) DARK_BG else LIGHT_BG
        val style = if (dark) SystemBarStyle.dark(Color.TRANSPARENT) else SystemBarStyle.light(Color.TRANSPARENT, 0x801B1B1B.toInt()) // the scrim only where icons cannot be dark (API 26)
        enableEdgeToEdge(statusBarStyle = style, navigationBarStyle = style)
        root.setBackgroundColor(bg)
        window.decorView.setBackgroundColor(bg)
        if (::web.isInitialized) web.setBackgroundColor(bg)
        errorView.setBackgroundColor(bg)
        errorView.findViewById<android.widget.TextView>(R.id.error_title).setTextColor(if (dark) 0xFFE8EAED.toInt() else 0xFF1D2329.toInt())
        errorView.findViewById<android.widget.TextView>(R.id.error_text).setTextColor(if (dark) 0xFF9AA4AE.toInt() else 0xFF5D6974.toInt())
    }

    private fun keepScreenOn(on: Boolean) {
        if (on) window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        else window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
    }

    private fun releaseWakeLocks() {
        wakeLocks.clear()
        keepScreenOn(false)
    }

    // ---------------------------------------------------------------- page, error, retry
    private fun showPage() {
        errorView.visibility = View.GONE
        web.visibility = View.VISIBLE
        ready = true
    }

    private fun showError(url: String) {
        pageFailed = true
        failedUrl = url
        web.visibility = View.INVISIBLE
        errorView.visibility = View.VISIBLE
        ready = true
    }

    private fun retry() {
        val url = failedUrl ?: HOME
        errorView.visibility = View.GONE
        web.loadUrl(url)
    }

    // ---------------------------------------------------------------- links and intents
    private fun openOutside(uri: Uri) {
        val scheme = uri.scheme?.lowercase() ?: return
        if (scheme in BLOCKED_SCHEMES) return
        val intent = if (scheme == "intent") {
            try {
                // as Chrome does: only activities that take links, no explicit target, no URI grants
                Intent.parseUri(uri.toString(), Intent.URI_INTENT_SCHEME).apply {
                    addCategory(Intent.CATEGORY_BROWSABLE)
                    component = null
                    selector = null
                    flags = 0
                    clipData = null
                }
            } catch (e: Exception) {
                return
            }
        } else {
            Intent(Intent.ACTION_VIEW, uri).addCategory(Intent.CATEGORY_BROWSABLE)
        }
        if (intent.data?.scheme?.lowercase() in BLOCKED_SCHEMES) return
        try {
            startActivity(intent)
        } catch (e: ActivityNotFoundException) {
            // the fallback of an intent: link, a web page only
            val fallback = intent.getStringExtra("browser_fallback_url")?.let { Uri.parse(it) } ?: return
            if (isOurs(fallback)) web.loadUrl(fallback.toString())
            else if (fallback.scheme == "https" || fallback.scheme == "http") openOutside(fallback)
        } catch (e: RuntimeException) {
            // SecurityException and the like: the link is not opened
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        if (intent.action != Intent.ACTION_VIEW) return
        val link = intent.data?.toString()?.let(::linkOf)
        if (link != null) open(link) else openLater(intent)
    }

    private fun open(link: String) {
        clearHistoryWhenLoaded = true
        web.loadUrl(link)
    }

    // the page of a link: one of hippomaps.ch as it is, a map link of another app or site (geo:,
    // Google Maps, OpenStreetMap, Apple Maps) as the place, route or search of hippomaps.ch
    private fun linkOf(s: String): String? =
        if (isOurs(Uri.parse(s))) s else MapLinks.convert(s)?.takeIf { isOurs(Uri.parse(it)) }

    // a short link (maps.app.goo.gl) leads to its map link through redirects, asked here; anything
    // else not understood says so, and the map stays as it is
    private fun openLater(intent: Intent) {
        if (intent.action != Intent.ACTION_VIEW) return
        val s = intent.data?.toString() ?: return
        if (!MapLinks.isShort(s)) return notUnderstood()
        thread(name = "short-link") {
            val link = resolve(s)?.let(::linkOf)
            runOnUiThread {
                if (isDestroyed) return@runOnUiThread
                if (link != null) open(link) else notUnderstood()
            }
        }
    }

    private fun notUnderstood() = Toast.makeText(this, R.string.link_unknown, Toast.LENGTH_LONG).show()

    // follows up to six redirects of https links, without loading their pages; the consent page of
    // Google carries the link in continue=
    private fun resolve(start: String): String? {
        var link = start
        repeat(6) {
            if (MapLinks.convert(link) != null && !MapLinks.isShort(link)) return link
            val u = MapLinks.parse(link) ?: return null
            if (u.host == "consent.google.com") return u.params["continue"]
            if (!link.startsWith("https://", ignoreCase = true)) return null
            val c = try {
                (URL(link).openConnection() as HttpURLConnection).apply {
                    instanceFollowRedirects = false
                    connectTimeout = 6000
                    readTimeout = 6000
                    setRequestProperty("User-Agent", "hippomaps-android")
                }
            } catch (e: Exception) { return null }
            try {
                if (c.responseCode !in 300..399) return null
                link = URL(URL(link), c.getHeaderField("Location") ?: return null).toString()
            } catch (e: Exception) {
                return null
            } finally {
                c.disconnect()
            }
        }
        return null
    }

    // ---------------------------------------------------------------- back
    private fun back(callback: OnBackPressedCallback) {
        if (errorView.visibility == View.VISIBLE || !ready) return leave(callback)
        web.evaluateJavascript("window.__hmBack ? window.__hmBack() : 'none'") { result ->
            when (result?.trim('"')) {
                "handled" -> Unit
                "nav" -> moveTaskToBack(true) // navigating: the app goes to the background, navigation is not ended
                else -> if (web.canGoBack()) web.goBack() else leave(callback)
            }
        }
    }

    private fun leave(callback: OnBackPressedCallback) {
        callback.isEnabled = false
        onBackPressedDispatcher.onBackPressed()
        callback.isEnabled = true
    }

    // ---------------------------------------------------------------- lifecycle
    override fun onResume() {
        super.onResume()
        if (!webGone) web.onResume()
    }

    override fun onStart() {
        super.onStart()
        if (geoWanted && hasLocation()) startFeed()
        if (compassWanted) compass.start()
        // allowed meanwhile in the settings, or the service did not start: navigation goes on in the background
        if (navigating && hasLocation() && NavigationService.instance == null) startNavigationService()
    }

    override fun onStop() {
        // positions in the background only for navigation
        if (!navigating) feed.stop()
        compass.stop() // the heading only for the page in view
        super.onStop()
    }

    override fun onPause() {
        // while navigating the page goes on in the background: positions, timers, the voice
        if (!navigating && !webGone) web.onPause()
        CookieManager.getInstance().flush()
        super.onPause()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        if (!webGone) web.saveState(outState)
    }

    override fun onDestroy() {
        feed.stop()
        compass.stop()
        NavigationService.onStopRequested = null
        NavigationService.stop(this)
        try {
            getSystemService(ConnectivityManager::class.java)?.unregisterNetworkCallback(network)
        } catch (_: IllegalArgumentException) {
        }
        handler.removeCallbacksAndMessages(null)
        if (!webGone) {
            root.removeView(web)
            web.destroy()
        }
        super.onDestroy()
    }

    // the turn of the screen against the phone (Surface.ROTATION_*), for the compass
    @Suppress("DEPRECATION")
    private fun screenRotation(): Int =
        (if (Build.VERSION.SDK_INT >= 30) display else windowManager.defaultDisplay)?.rotation ?: 0

    private fun hasLocation(): Boolean =
        ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED ||
            ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_COARSE_LOCATION) == PackageManager.PERMISSION_GRANTED

    private fun isSystemDark(): Boolean =
        (resources.configuration.uiMode and android.content.res.Configuration.UI_MODE_NIGHT_MASK) ==
            android.content.res.Configuration.UI_MODE_NIGHT_YES
}
