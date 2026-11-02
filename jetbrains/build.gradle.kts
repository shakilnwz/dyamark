plugins {
    id("org.jetbrains.kotlin.jvm") version "2.4.21"
    id("org.jetbrains.intellij.platform") version "2.19.0"
}

group = "io.github.shakilnwz.dyamark"
version = providers.gradleProperty("pluginVersion").orNull
    ?: providers.environmentVariable("VERSION").orNull
    ?: "0.1.0-dev"

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

dependencies {
    intellijPlatform {
        val localIde = providers.gradleProperty("dyamark.ide").orNull?.let { file(it) }
        if (localIde != null && localIde.isDirectory) {
            local(localIde)
        } else {
            phpstorm("2026.2.4")
        }
        bundledPlugin("org.intellij.plugins.markdown")
        bundledPlugin("com.intellij.modules.jcef")
    }
    testImplementation(kotlin("test"))
}

kotlin {
    jvmToolchain(25)
}

intellijPlatform {
    buildSearchableOptions = false
    instrumentCode = false
    pluginConfiguration {
        ideaVersion {
            sinceBuild = "262"
            untilBuild = "262.*"
        }
    }
}

// The web build (bin/wasm) goes into the jar under web/.
tasks.processResources {
    from(rootDir.resolve("../build/web")) {
        into("web")
    }
}

// Spike runs: -Pspike opens spike.md from -PspikeProject in dyamark's
// preview, screenshots it, types into it, presses keys in it and logs what
// happened. -PnoGpu runs JCEF with the GPU off, -PjcefArgs passes Chromium
// flags, and -PdevtoolsPort opens Chromium's DevTools port.
val runIdeArgs = buildList {
    add("-Didea.trust.all.projects=true")
    add("-Djb.consents.confirmation.enabled=false")
    add("-Djb.privacy.policy.text=<!--999.999-->")
    add("-Dide.show.tips.on.startup.default.value=false")
    add("-Didea.initially.ask.config=never")
    if (providers.gradleProperty("spike").isPresent) add("-Ddyamark.spike=true")
    if (providers.gradleProperty("noGpu").isPresent) add("-Dide.browser.jcef.gpu.disable=true")
    providers.gradleProperty("devtoolsPort").orNull?.let { add("-Dide.browser.jcef.debug.port=$it") }
    providers.gradleProperty("jcefArgs").orNull?.let { add("-Dide.browser.jcef.extra.args=$it") }
}
val spikeProject = providers.gradleProperty("spikeProject").orNull

tasks.runIde {
    jvmArgs(runIdeArgs)
    if (spikeProject != null) args(spikeProject)
}
