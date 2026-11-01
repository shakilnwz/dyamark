package dyamark.jetbrains

import com.intellij.openapi.diagnostic.logger
import org.cef.browser.CefBrowser
import org.cef.browser.CefFrame
import org.cef.callback.CefCallback
import org.cef.handler.CefRequestHandlerAdapter
import org.cef.handler.CefResourceHandler
import org.cef.handler.CefResourceHandlerAdapter
import org.cef.handler.CefResourceRequestHandler
import org.cef.handler.CefResourceRequestHandlerAdapter
import org.cef.misc.BoolRef
import org.cef.misc.IntRef
import org.cef.misc.StringRef
import org.cef.network.CefRequest
import org.cef.network.CefResponse

/**
 * Serves the web build (index.html, wasm_exec.js, dyamark.wasm) from the
 * plugin's jar at [ORIGIN], and refuses every other request: the page
 * needs nothing from the network.
 */
object WebResources : CefRequestHandlerAdapter() {
    private val LOG = logger<WebResources>()

    const val ORIGIN = "https://dyamark.local/"
    const val INDEX = ORIGIN + "index.html"

    private val files = mutableMapOf<String, ByteArray?>()

    private fun bytes(path: String): ByteArray? = synchronized(files) {
        files.getOrPut(path) {
            WebResources::class.java.getResourceAsStream("/web/$path")?.use { it.readAllBytes() }
        }
    }

    override fun getResourceRequestHandler(
        browser: CefBrowser?,
        frame: CefFrame?,
        request: CefRequest,
        isNavigation: Boolean,
        isDownload: Boolean,
        requestInitiator: String?,
        disableDefaultHandling: BoolRef?,
    ): CefResourceRequestHandler = object : CefResourceRequestHandlerAdapter() {
        override fun onBeforeResourceLoad(browser: CefBrowser?, frame: CefFrame?, request: CefRequest): Boolean =
            !request.getURL().startsWith(ORIGIN)

        override fun getResourceHandler(browser: CefBrowser?, frame: CefFrame?, request: CefRequest): CefResourceHandler? {
            val url = request.getURL()
            if (!url.startsWith(ORIGIN)) return null
            val path = url.removePrefix(ORIGIN).substringBefore('?').substringBefore('#')
            val data = bytes(path)
            LOG.info("dyamark: serving $path (${data?.size ?: "missing"})")
            return Resource(path, data)
        }
    }

    private class Resource(private val path: String, private val data: ByteArray?) : CefResourceHandlerAdapter() {
        private var sent = 0

        override fun processRequest(request: CefRequest, callback: CefCallback): Boolean {
            callback.Continue()
            return true
        }

        override fun getResponseHeaders(response: CefResponse, length: IntRef, redirectUrl: StringRef) {
            if (data == null) {
                response.status = 404
                response.statusText = "Not Found"
                length.set(0)
                return
            }
            response.status = 200
            response.statusText = "OK"
            response.mimeType = mimeType(path)
            length.set(data.size)
        }

        override fun readResponse(out: ByteArray, bytesToRead: Int, bytesRead: IntRef, callback: CefCallback): Boolean {
            val data = data
            if (data == null || sent >= data.size) {
                bytesRead.set(0)
                return false
            }
            val n = minOf(bytesToRead, data.size - sent)
            System.arraycopy(data, sent, out, 0, n)
            sent += n
            bytesRead.set(n)
            return true
        }

        private fun mimeType(path: String) = when (path.substringAfterLast('.')) {
            "html" -> "text/html"
            "js" -> "text/javascript"
            "wasm" -> "application/wasm"
            else -> "application/octet-stream"
        }
    }
}
