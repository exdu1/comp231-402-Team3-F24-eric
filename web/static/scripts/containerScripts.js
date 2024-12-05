async function editField(button) {
	var row = button.parentNode.parentNode;
	var fieldName = row.cells[0].innerText;

	// Get field value from backend
	var fieldValue = row.cells[1].innerText; // TODO: fetch from backend

	var newFieldValue = prompt("Edit " + fieldName + ":", fieldValue);

	if (newFieldValue !== null) {
		row.cells[1].innerText = newFieldValue;
		// TODO: Send updated field value to backend
	}
}

async function deleteContainer() {
	if (confirm("Are you sure you want to delete this container?")) {
		// TODO: Send delete request to backend
	}
}

async function checkContainerState() {
	var btn = document.getElementById("btnStartStop");
	// TODO: Fetch container state from backend

	if (true) {
		btn.innerText = "Stop Container";
	} else {
		btn.innerText = "Start Container";
	}
}

async function toggleStartStopContainer() {
	var btn = document.getElementById("btnStartStop");
	// TODO: get container state from backend

	if (btn.innerText === "Start Container") {
		// TODO: Send start request to backend
		btn.innerText = "Stop Container";
	} else {
		// TODO: Send stop request to backend
		btn.innerText = "Start Container";
	}
}


// Container Creation Scripts