
var Conn = null
var Settings = {
	Token: "DEADBEEF",
}


var cmdpointer = 0, cmdbuffer = [];
var cmdhistory =  {
	get: function(key){
		if (key < 0){
			return cmdbuffer[cmdpointer+key];
		} else if (key === false) {
			return cmdbuffer[cmdpointer - 1];
		} else {
			return cmdbuffer[key];
		}
	},
	push: function(item){
		cmdbuffer[cmdpointer] = item;
		cmdpointer = (cmdpointer + 1) % 32;
		return item;
	},
	prev: function(){
		var tmp_pointer = (cmdpointer - 1) % 32;
		if (cmdbuffer[tmp_pointer]){
			cmdpointer = tmp_pointer;
			return cmdbuffer[cmdpointer];
		}
	},
	next: function(){
		if (cmdbuffer[cmdpointer]){
			cmdpointer = (cmdpointer + 1) % 32;
			return cmdbuffer[cmdpointer];
		}
	}
}

$(document).ready(function() {
	var savedata = window.localStorage["VS.ServerMonitor.Settings"]
	if (savedata) {
		var lset = JSON.parse(savedata)
		Settings.Token = lset.Token !== undefined ? lset.Token : Settings.Token
	}

	function makeTab(sid, name) {
		// If tab already exists in the list, return
		if ($(`#${sid}`).length != 0) {
			return
		}
		
		// hide other tabs
		$("#tabs li").removeClass("current")
		$("#content div.tab").hide()
		
		// add new tab and related content
		$("#tabs").append(`<li class='current'><a class='tab' id='${sid}' href='#'>${name}</a></li>`)

		$("#content").append("<div class='tab' id='" + sid + "'><div class='output'></div><div class='bottomrow'>" +
			"<input class='enter' value='>>>' type='button'/><input class='commandline' type='input' /></div></div>")
		
		// set the newly added tab as current
		$(`#${sid}`).show()
	}

	$('#tabs').on('click', "a.tab", function() {
		// Get the tab name
		var sid = $(this).attr("id")

		// hide all other tabs
		$("#content div.tab").hide()
		$("#tabs li").removeClass("current")

		// show current tab
		$(`#content #${sid}`).show()
		$(this).parent().addClass("current")
	})

	$('#content').on('click', "div.tab", function() {
		if (window.getSelection().type == "Range") {
			return
		}

		$(this).find(".commandline").focus()
	})

	$('#content').on('click', "input.enter", function() {
		var sid = $(this).parents("div.tab").attr("id")
		var el = $(`#content #${sid} input.commandline`)[0]
		Conn.send(`{"SID": ${sid}, "Token": "${Settings.Token}", "Command": ${JSON.stringify(el.value)}}`)
		cmdhistory.push(el.value)
		el.value = ""
	})

	$('#content').on('keydown', "input.commandline", function(evnt) {
		var sid = $(this).parents("div.tab").attr("id")
		if (evnt.which == 13) {
			var el = $(`#content #${sid} input.commandline`)[0]
			Conn.send(`{"SID": ${sid}, "Token": "${Settings.Token}", "Command": ${JSON.stringify(el.value)}}`)
			cmdhistory.push(el.value)
			el.value = ""
		} else if (evnt.which == 38) {
			var v = cmdhistory.prev()
			$(`#content #${sid} input.commandline`)[0].value = v === undefined ? "" : v
		} else if (evnt.which == 40) {
			var v = cmdhistory.next()
			$(`#content #${sid} input.commandline`)[0].value = v === undefined ? "" : v
		}
	})

	Conn = new ReconnectingWebSocket((window.location.protocol == "https:" ? "wss://" : "ws://")+window.location.host+"/socket")
	Conn.onmessage = function(evnt) {
		var msg = JSON.parse(evnt.data)
		
		if (msg.Class == "Monitor Init") {
			makeTab(msg.SID, msg.Message)
			return
		}

		msg.At = new Date(msg.At).toLocaleTimeString()

		var el = $(`#content #${msg.SID} .output`)
		if (el.length == 0) {
			return
		}

		el.append(`<div><span class="log-time">${msg.At}</span> <span class="log-class log-${msg.Class.replace(" ", "-")}">[${msg.Class}]:</span> <span class="log-message">${msg.Message}</span></div>`)
		
		// Don't autoscroll if the user isn't at the bottom.
		var at = el.scrollTop() + el.innerHeight()
		el = el[0]
		if (at >= el.scrollHeight - 60) {
			el.scrollTop = el.scrollHeight
		}
	}
	Conn.onopen = function(evnt) {
		Conn.send(`{"SID": 0, "Token": "${Settings.Token}", "Command": ":auth"}`)
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
		window.localStorage["VS.ServerMonitor.Settings"] = JSON.stringify(Settings)
		$("#SettingsBox").hide()
	})

	$("#HelpPanel").hide()
	$("#ShowHelp").click(function(evnt) {
		evnt.stopPropagation()
		$("#HelpPanel").show()
	})
	$("#helpOK").click(function(evnt) {
		evnt.stopPropagation()
		$("#HelpPanel").hide()
	})
})
