
var Conn = null
var Settings = {
	Token: "DEADBEEF",
}

$(document).ready(function() {
	var savedata = window.localStorage["VS.ServerMonitor.Settings"]
	if (savedata) {
		var lset = JSON.parse(savedata)
		Settings.Token = lset.Token !== undefined ? lset.Token : Settings.Token
	}

	Conn = new ReconnectingWebSocket((window.location.protocol == "https:" ? "wss://" : "ws://")+window.location.host+"/socket")
	Conn.onmessage = function(evnt) {
		var el = $('#Output')[0]
		el.value += evnt.data
		el.scrollTop = el.scrollHeight
	}

	$("#SettingsBox").hide()
	$("#ShowSettings").click(function(evnt) {
		evnt.stopPropagation()
		$("#sToken")[0].value = Settings.Token
		$("#SettingsBox").show()
	})
	$("#sCancel").click(function(evnt) {
		evnt.stopPropagation()
		$("#SettingsBox").hide()
	})
	$("#sOK").click(function(evnt) {
		evnt.stopPropagation()
		Settings.Token = $("#sToken")[0].value
		$("#SettingsBox").hide()
	})

	$("#Command").on("keypress", function(evnt) {
		if (evnt.which == 13) {
			var el = $("#Command")[0]
			Conn.send(`${Settings.Token}|${el.value}\n`)
			el.value = ""
		}
	})
	$("#Enter").on("click", function(evnt) {
		var el = $("#Command")[0]
		Conn.send(`${Settings.Token}|${el.value}\n`)
		el.value = ""
	})
})
